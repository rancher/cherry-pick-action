package orchestrator_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/cherry-pick-action/internal/git"
	gh "github.com/rancher/cherry-pick-action/internal/github"
	"github.com/rancher/cherry-pick-action/internal/orchestrator"
)

type fakeGHClient struct {
	pr             gh.PRMetadata
	prResponses    []gh.PRMetadata
	err            error
	branches       map[string]bool
	existingPR     map[string][]gh.CherryPickPR
	commits        map[string]map[string]bool
	createPRReturn gh.CherryPickPR
	createPRErr    error
	createPRInputs []gh.CreatePROptions
	prCallCount    int
	comments       map[int][]gh.IssueComment
	updateErrors   map[int64]error
	updated        []gh.IssueComment
	labels         map[string]bool // Track labels for HasLabel checks
	addedLabels    []string        // Track AddLabel calls
}

func (f *fakeGHClient) GetPullRequest(_ context.Context, owner, repo string, number int) (gh.PRMetadata, error) {
	f.prCallCount++
	if f.err != nil {
		return gh.PRMetadata{}, f.err
	}

	if len(f.prResponses) > 0 {
		idx := f.prCallCount - 1
		if idx >= len(f.prResponses) {
			idx = len(f.prResponses) - 1
		}
		return f.prResponses[idx], nil
	}

	return f.pr, nil
}

func (f *fakeGHClient) ListCherryPickPRs(_ context.Context, owner, repo string, prNumber int, branch string) ([]gh.CherryPickPR, error) {
	if f.existingPR == nil {
		return nil, nil
	}
	return f.existingPR[branch], nil
}

func (f *fakeGHClient) EnsureBranchExists(_ context.Context, owner, repo, branch string) error {
	if f.branches == nil {
		return nil
	}
	if exists, ok := f.branches[branch]; ok {
		if !exists {
			return gh.ErrBranchNotFound
		}
		return nil
	}
	return nil
}

func (f *fakeGHClient) CreateBranch(context.Context, string, string, string, string) error {
	return nil
}

func (f *fakeGHClient) CreatePullRequest(_ context.Context, owner, repo string, input gh.CreatePROptions) (gh.CherryPickPR, error) {
	f.createPRInputs = append(f.createPRInputs, input)
	if f.createPRErr != nil {
		return gh.CherryPickPR{}, f.createPRErr
	}
	result := f.createPRReturn
	if result.Number == 0 && result.URL == "" {
		result = gh.CherryPickPR{
			URL:    "https://example.com/pr",
			Number: len(f.createPRInputs),
			Head:   input.Head,
			Base:   input.Base,
		}
	}
	return result, nil
}

func (f *fakeGHClient) CommentOnPullRequest(context.Context, string, string, int, string) error {
	return nil
}

func (f *fakeGHClient) CommitExistsOnBranch(_ context.Context, owner, repo, commitSHA, branch string) (bool, error) {
	if f.commits == nil {
		return false, nil
	}

	if branchCommits, ok := f.commits[branch]; ok {
		return branchCommits[commitSHA], nil
	}

	return false, nil
}

func (f *fakeGHClient) ListPullRequestComments(_ context.Context, _ string, _ string, number int) ([]gh.IssueComment, error) {
	if f.comments == nil {
		return nil, nil
	}
	return f.comments[number], nil
}

func (f *fakeGHClient) UpdateComment(_ context.Context, _ string, _ string, commentID int64, body string) error {
	if f.updateErrors != nil {
		if err, ok := f.updateErrors[commentID]; ok {
			return err
		}
	}
	f.updated = append(f.updated, gh.IssueComment{ID: commentID, Body: body})
	return nil
}

func (f *fakeGHClient) HasLabel(_ context.Context, owner, repo string, number int, label string) (bool, error) {
	if f.labels == nil {
		return false, nil
	}
	return f.labels[label], nil
}

func (f *fakeGHClient) AddLabel(_ context.Context, owner, repo string, number int, label string) error {
	f.addedLabels = append(f.addedLabels, label)
	return nil
}

func (f *fakeGHClient) CheckOrgMembership(_ context.Context, org, username string) (bool, error) {
	// Default implementation: user is a member
	return true, nil
}

var _ = Describe("Orchestrator", func() {
	var (
		ctx context.Context
		cfg orchestrator.Config
	)

	BeforeEach(func() {
		ctx = context.Background()
		cfg = orchestrator.Config{LabelPrefix: "cherry-pick/", ConflictStrategy: "fail", DryRun: true}
	})

	It("skips processing when the pull request is not merged", func() {
		client := &fakeGHClient{pr: gh.PRMetadata{IsMerged: false}}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeTrue())
		Expect(result.SkippedReason).To(Equal("not merged"))
	})

	It("skips when no matching labels are present", func() {
		client := &fakeGHClient{pr: gh.PRMetadata{IsMerged: true, Labels: []string{"kind/bug"}}}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 2)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeTrue())
		Expect(result.SkippedReason).To(Equal("no targets"))
	})

	It("uses configured target branches when labels are absent", func() {
		cfg.TargetBranches = []string{"release/v0.30", "release/v0.29"}
		client := &fakeGHClient{
			pr: gh.PRMetadata{IsMerged: true, MergeSHA: "abc123"},
			branches: map[string]bool{
				"release/v0.30": true,
				"release/v0.29": true,
			},
		}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeFalse())
		Expect(result.Targets).To(HaveLen(2))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusDryRun))
		Expect(result.Targets[1].Status).To(Equal(orchestrator.TargetStatusDryRun))
		Expect(result.Targets[0].Target.Branch).To(Equal("release/v0.30"))
		Expect(result.Targets[1].Target.Branch).To(Equal("release/v0.29"))
		Expect(client.prCallCount).To(Equal(2))
	})

	It("deduplicates overrides that match label-derived targets", func() {
		cfg.TargetBranches = []string{"release/v0.25"}
		client := &fakeGHClient{pr: gh.PRMetadata{
			IsMerged: true,
			Labels:   []string{"cherry-pick/release/v0.25"},
			MergeSHA: "abc123",
		}, branches: map[string]bool{"release/v0.25": true}}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 21)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeFalse())
		Expect(result.Targets).To(HaveLen(1))
		Expect(result.Targets[0].Target.LabelName).To(Equal("cherry-pick/release/v0.25"))
	})

	It("deduplicates duplicate label targets", func() {
		cfg.DryRun = true
		client := &fakeGHClient{pr: gh.PRMetadata{
			IsMerged: true,
			Labels:   []string{"cherry-pick/release/v0.25", "cherry-pick/release/v0.25"},
			MergeSHA: "abc123",
		}, branches: map[string]bool{
			"release/v0.25": true,
		}}

		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 22)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(1))
		Expect(result.Targets[0].Target.Branch).To(Equal("release/v0.25"))
	})

	It("returns dry-run statuses when valid labels are present", func() {
		client := &fakeGHClient{pr: gh.PRMetadata{
			IsMerged: true,
			Labels:   []string{"cherry-pick/release/v0.25", "cherry-pick/release/v0.24", "other"},
			MergeSHA: "abc123",
		}, branches: map[string]bool{
			"release/v0.25": true,
			"release/v0.24": true,
		}}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 3)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeFalse())
		Expect(result.Targets).To(HaveLen(2))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusDryRun))
		Expect(result.Targets[1].Status).To(Equal(orchestrator.TargetStatusDryRun))
	})

	It("marks targets as skipped when branch is missing", func() {
		client := &fakeGHClient{pr: gh.PRMetadata{
			IsMerged: true,
			Labels:   []string{"cherry-pick/release/v0.25", "cherry-pick/release/v0.24"},
			MergeSHA: "abc123",
		}, branches: map[string]bool{
			"release/v0.25": false,
			"release/v0.24": true,
		}}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 4)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(2))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusSkippedNoBranch))
		Expect(result.Targets[0].Reason).To(ContainSubstring("target branch not found"))
		Expect(result.Targets[1].Status).To(Equal(orchestrator.TargetStatusDryRun))
	})

	It("skips forked pull requests with actionable messaging", func() {
		client := &fakeGHClient{pr: gh.PRMetadata{
			IsMerged:   true,
			MergeSHA:   "abc123",
			IsFromFork: true,
			HeadOwner:  "contributor",
			HeadRepo:   "repo-fork",
		}}

		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 30)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeTrue())
		Expect(result.SkippedReason).To(ContainSubstring("fork"))
		Expect(result.SkippedReason).To(ContainSubstring("create a branch"))
	})

	It("marks targets as skipped when an existing cherry-pick PR is found", func() {
		existingPR := gh.CherryPickPR{URL: "https://github.com/rancher/repo/pull/10", Number: 10, Head: "cherry-pick/release/v0.25/pr-1", Base: "release/v0.25"}
		client := &fakeGHClient{pr: gh.PRMetadata{
			IsMerged: true,
			Labels:   []string{"cherry-pick/release/v0.25"},
			MergeSHA: "abc123",
		}, branches: map[string]bool{"release/v0.25": true}, existingPR: map[string][]gh.CherryPickPR{
			"release/v0.25": {existingPR},
		}}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 5)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(1))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusSkippedExistingPR))
		Expect(result.Targets[0].ExistingPR).NotTo(BeNil())
		Expect(result.Targets[0].ExistingPR.URL).To(Equal(existingPR.URL))
	})

	It("skips targets when commit already exists on branch", func() {
		client := &fakeGHClient{pr: gh.PRMetadata{
			IsMerged: true,
			Labels:   []string{"cherry-pick/release/v0.25"},
			MergeSHA: "abc123",
		}, branches: map[string]bool{"release/v0.25": true}, commits: map[string]map[string]bool{
			"release/v0.25": {"abc123": true},
		}}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 6)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(1))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusSkippedAlreadyPresent))
	})

	It("re-reads labels before execution to handle stale targets", func() {
		first := gh.PRMetadata{
			IsMerged: true,
			Labels:   []string{"cherry-pick/release/v0.25"},
			MergeSHA: "abc123",
		}
		second := first
		second.Labels = nil

		client := &fakeGHClient{
			prResponses: []gh.PRMetadata{first, second},
			branches:    map[string]bool{"release/v0.25": true},
		}
		orch := orchestrator.New(cfg, client, nil, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 7)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeTrue())
		Expect(result.SkippedReason).To(Equal("no targets"))
		Expect(client.prCallCount).To(Equal(2))
	})

	It("creates cherry-pick PRs for pending targets", func() {
		cfg.DryRun = false
		client := &fakeGHClient{pr: gh.PRMetadata{
			Owner:     "rancher",
			Repo:      "repo",
			Number:    7,
			Title:     "Fix critical bug",
			Body:      "Original PR body",
			Labels:    []string{"cherry-pick/release/v0.25", "kind/bug"},
			Assignees: []string{"alice"},
			MergeSHA:  "abc123",
			IsMerged:  true,
		}, branches: map[string]bool{
			"release/v0.25": true,
		}}
		client.createPRReturn = gh.CherryPickPR{URL: "https://github.com/rancher/repo/pull/99", Number: 99, Head: "cherry-pick/release/v0.25/pr-7", Base: "release/v0.25"}

		workspace := &fakeWorkspace{}
		gitExec := &fakeGitExecutor{workspace: workspace}

		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 7)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(1))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusSucceeded))
		Expect(result.Targets[0].CreatedPR).NotTo(BeNil())
		Expect(result.Targets[0].CreatedPR.Number).To(Equal(99))
		Expect(workspace.cleanupCalled).To(BeTrue())
		Expect(workspace.cherryPicks).To(ContainElement("abc123"))
		Expect(workspace.pushes).To(ContainElement("cherry-pick/release/v0.25/pr-7"))
		Expect(client.createPRInputs).To(HaveLen(1))
		Expect(client.createPRInputs[0].Head).To(Equal("cherry-pick/release/v0.25/pr-7"))
		Expect(client.createPRInputs[0].Labels).To(ConsistOf("kind/bug"))
		Expect(client.createPRInputs[0].Body).To(ContainSubstring("<!-- cherry-pick-of: rancher/repo#7 -> release/v0.25 -->"))
	})

	It("records failures when cherry-pick conflicts", func() {
		cfg.DryRun = false
		client := &fakeGHClient{pr: gh.PRMetadata{
			Owner:    "rancher",
			Repo:     "repo",
			Number:   8,
			Title:    "Feature",
			Labels:   []string{"cherry-pick/release/v0.25"},
			MergeSHA: "def456",
			IsMerged: true,
		}, branches: map[string]bool{
			"release/v0.25": true,
		}}

		workspace := &fakeWorkspace{cherryPickErr: errors.New("conflict")}
		gitExec := &fakeGitExecutor{workspace: workspace}

		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 8)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(1))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusFailed))
		Expect(result.Targets[0].Reason).To(ContainSubstring("cherry-pick commit"))
		Expect(workspace.abortCalled).To(BeTrue())
		Expect(workspace.cleanupCalled).To(BeTrue())
		Expect(client.createPRInputs).To(BeEmpty())
	})

	It("opens placeholder PRs when conflict strategy is placeholder-pr", func() {
		cfg.DryRun = false
		cfg.ConflictStrategy = "placeholder-pr"
		client := &fakeGHClient{pr: gh.PRMetadata{
			Owner:    "rancher",
			Repo:     "repo",
			Number:   9,
			Title:    "Hotfix",
			Labels:   []string{"cherry-pick/release/v0.25"},
			MergeSHA: "conflictsha",
			IsMerged: true,
		}, branches: map[string]bool{
			"release/v0.25": true,
		}}

		workspace := &fakeWorkspace{cherryPickErr: errors.New("conflict: manual resolution required")}
		gitExec := &fakeGitExecutor{workspace: workspace}
		client.createPRReturn = gh.CherryPickPR{URL: "https://github.com/rancher/repo/pull/100", Number: 100}

		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 9)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(1))
		target := result.Targets[0]
		Expect(target.Status).To(Equal(orchestrator.TargetStatusPlaceholderPR))
		Expect(target.CreatedPR).NotTo(BeNil())
		Expect(target.CreatedPR.Number).To(Equal(100))
		Expect(target.Reason).To(ContainSubstring("placeholder PR opened"))
		Expect(workspace.abortCalled).To(BeTrue())
		Expect(len(workspace.emptyCommits)).To(Equal(1))
		Expect(workspace.pushes).To(ContainElement(gh.BranchNameForCherryPick("release/v0.25", 9)))
		Expect(client.createPRInputs).To(HaveLen(1))
		Expect(client.createPRInputs[0].Body).To(ContainSubstring("encountered conflicts"))
		Expect(client.createPRInputs[0].Body).To(ContainSubstring("<!-- cherry-pick-of: rancher/repo#9 -> release/v0.25 -->"))
	})

	It("fails target when workspace preparation fails", func() {
		cfg.DryRun = false
		client := &fakeGHClient{pr: gh.PRMetadata{
			Owner:    "rancher",
			Repo:     "repo",
			Number:   10,
			Labels:   []string{"cherry-pick/release/v0.26"},
			MergeSHA: "abc123",
			IsMerged: true,
		}, branches: map[string]bool{"release/v0.26": true}}

		gitExec := &fakeGitExecutor{err: errors.New("prepare failed")}
		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Targets).To(HaveLen(1))
		target := result.Targets[0]
		Expect(target.Status).To(Equal(orchestrator.TargetStatusFailed))
		Expect(target.Reason).To(ContainSubstring("prepare workspace"))
	})

	It("fails target when pushing the cherry-pick branch fails", func() {
		cfg.DryRun = false
		client := &fakeGHClient{pr: gh.PRMetadata{
			Owner:    "rancher",
			Repo:     "repo",
			Number:   11,
			Title:    "Bugfix",
			Labels:   []string{"cherry-pick/release/v0.27"},
			MergeSHA: "abc123",
			IsMerged: true,
		}, branches: map[string]bool{"release/v0.27": true}}

		workspace := &fakeWorkspace{pushErr: errors.New("permission denied")}
		gitExec := &fakeGitExecutor{workspace: workspace}
		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 11)
		Expect(err).NotTo(HaveOccurred())
		target := result.Targets[0]
		Expect(target.Status).To(Equal(orchestrator.TargetStatusFailed))
		Expect(target.Reason).To(ContainSubstring("push cherry-pick branch"))
		Expect(target.Reason).To(ContainSubstring("permission denied"))
		Expect(workspace.cleanupCalled).To(BeTrue())
		Expect(client.createPRInputs).To(BeEmpty())
	})

	It("fails target when pull request creation fails", func() {
		cfg.DryRun = false
		client := &fakeGHClient{pr: gh.PRMetadata{
			Owner:    "rancher",
			Repo:     "repo",
			Number:   12,
			Title:    "Bugfix",
			Labels:   []string{"cherry-pick/release/v0.28"},
			MergeSHA: "abc123",
			IsMerged: true,
		}, branches: map[string]bool{"release/v0.28": true}, createPRErr: errors.New("API error")}

		workspace := &fakeWorkspace{}
		gitExec := &fakeGitExecutor{workspace: workspace}
		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 12)
		Expect(err).NotTo(HaveOccurred())
		target := result.Targets[0]
		Expect(target.Status).To(Equal(orchestrator.TargetStatusFailed))
		Expect(target.Reason).To(ContainSubstring("create pull request"))
		Expect(workspace.cleanupCalled).To(BeTrue())
	})

	It("skips target when cherry-pick-done label exists", func() {
		cfg.DryRun = false
		client := &fakeGHClient{
			pr: gh.PRMetadata{
				Owner:    "rancher",
				Repo:     "repo",
				Number:   1,
				MergeSHA: "abc123",
				Labels:   []string{"cherry-pick/release-v1.0"},
				IsMerged: true,
			},
			branches: map[string]bool{"release-v1.0": true},
			labels:   map[string]bool{"cherry-pick/done/release-v1.0": true},
		}

		workspace := &fakeWorkspace{}
		gitExec := &fakeGitExecutor{workspace: workspace}
		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 1)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeFalse())
		Expect(len(result.Targets)).To(Equal(1))
		// Target should be skipped due to done label (uses SkippedExistingPR status)
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusSkippedExistingPR))
		Expect(result.Targets[0].Reason).To(ContainSubstring("already cherry-picked"))
		// No git operations should occur
		Expect(workspace.prepareCalls).To(Equal(0))
	})

	It("adds cherry-pick-done label after successful PR creation", func() {
		cfg.DryRun = false
		client := &fakeGHClient{
			pr: gh.PRMetadata{
				Owner:    "rancher",
				Repo:     "repo",
				Number:   1,
				MergeSHA: "abc123",
				Labels:   []string{"cherry-pick/release-v1.0"},
				IsMerged: true,
			},
			branches:    map[string]bool{"release-v1.0": true},
			addedLabels: []string{}, // Track added labels
		}

		workspace := &fakeWorkspace{}
		gitExec := &fakeGitExecutor{workspace: workspace}
		orch := orchestrator.New(cfg, client, gitExec, nil)

		result, err := orch.ProcessPullRequest(ctx, "rancher", "repo", 1)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Skipped).To(BeFalse())
		Expect(len(result.Targets)).To(Equal(1))
		Expect(result.Targets[0].Status).To(Equal(orchestrator.TargetStatusSucceeded))

		// Verify that the done label was added
		Expect(client.addedLabels).To(ContainElement("cherry-pick/done/release-v1.0"))
	})
})

type fakeGitExecutor struct {
	workspace *fakeWorkspace
	err       error
}

func (f *fakeGitExecutor) Prepare(ctx context.Context, owner, repo string) (git.Workspace, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.workspace == nil {
		return nil, errors.New("workspace not configured")
	}
	f.workspace.prepareCalls++
	return f.workspace, nil
}

type fakeWorkspace struct {
	prepareCalls  int
	checkouts     []string
	created       []branchCall
	cherryPicks   []string
	pushes        []string
	emptyCommits  []string
	abortCalled   bool
	cleanupCalled bool

	checkoutErr     error
	createBranchErr error
	cherryPickErr   error
	pushErr         error
	abortErr        error
	emptyCommitErr  error
	cleanupErr      error
}

type branchCall struct {
	branch string
	from   string
}

func (w *fakeWorkspace) CheckoutBranch(ctx context.Context, branch string) error {
	w.checkouts = append(w.checkouts, branch)
	if w.checkoutErr != nil {
		return w.checkoutErr
	}
	return nil
}

func (w *fakeWorkspace) CreateBranchFrom(ctx context.Context, branch, from string) error {
	w.created = append(w.created, branchCall{branch: branch, from: from})
	if w.createBranchErr != nil {
		return w.createBranchErr
	}
	return nil
}

func (w *fakeWorkspace) CherryPick(ctx context.Context, commit string) error {
	w.cherryPicks = append(w.cherryPicks, commit)
	if w.cherryPickErr != nil {
		return w.cherryPickErr
	}
	return nil
}

func (w *fakeWorkspace) AbortCherryPick(ctx context.Context) error {
	w.abortCalled = true
	if w.abortErr != nil {
		return w.abortErr
	}
	return nil
}

func (w *fakeWorkspace) CommitAllowEmpty(ctx context.Context, message string) error {
	w.emptyCommits = append(w.emptyCommits, message)
	if w.emptyCommitErr != nil {
		return w.emptyCommitErr
	}
	return nil
}

func (w *fakeWorkspace) PushBranch(ctx context.Context, branch string) error {
	w.pushes = append(w.pushes, branch)
	if w.pushErr != nil {
		return w.pushErr
	}
	return nil
}

func (w *fakeWorkspace) Cleanup(ctx context.Context) error {
	w.cleanupCalled = true
	if w.cleanupErr != nil {
		return w.cleanupErr
	}
	return nil
}
