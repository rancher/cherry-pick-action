package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/rancher/cherry-pick-action/internal/git"
	gh "github.com/rancher/cherry-pick-action/internal/github"
	"github.com/rancher/cherry-pick-action/internal/labels"
)

// Orchestrator coordinates GitHub metadata, git operations, and label parsing to
// generate cherry-pick pull requests.
type Orchestrator struct {
	cfg Config
	gh  gh.Client
	git git.Executor
	log *slog.Logger
}

// TargetStatus describes the evaluation state for a target branch.
type TargetStatus string

const (
	TargetStatusPending               TargetStatus = "pending"
	TargetStatusDryRun                TargetStatus = "dry_run"
	TargetStatusSucceeded             TargetStatus = "succeeded"
	TargetStatusFailed                TargetStatus = "failed"
	TargetStatusPlaceholderPR         TargetStatus = "placeholder_pr"
	TargetStatusSkippedNoBranch       TargetStatus = "skipped_missing_branch"
	TargetStatusSkippedExistingPR     TargetStatus = "skipped_existing_pr"
	TargetStatusSkippedAlreadyPresent TargetStatus = "skipped_commit_present"
)

const (
	conflictStrategyFail          = "fail"
	conflictStrategyPlaceholderPR = "placeholder-pr"
)

// TargetResult captures per-target orchestration outcomes.
type TargetResult struct {
	Target     labels.Target
	Status     TargetStatus
	Reason     string
	ExistingPR *gh.CherryPickPR
	CreatedPR  *gh.CherryPickPR
}

// Result captures the outcome of a single orchestrator run.
type Result struct {
	Targets       []TargetResult
	Skipped       bool
	SkippedReason string
}

// New returns a configured Orchestrator instance.
func New(cfg Config, ghClient gh.Client, gitExecutor git.Executor, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{cfg: cfg, gh: ghClient, git: gitExecutor, log: logger}
}

// ProcessPullRequest evaluates a pull request and determines whether cherry-pick
// work should be performed. A best-effort Result is always returned when err == nil.
func (o *Orchestrator) ProcessPullRequest(ctx context.Context, owner, repo string, number int) (Result, error) {
	if o.gh == nil {
		return Result{}, fmt.Errorf("github client is required")
	}

	pr, err := o.gh.GetPullRequest(ctx, owner, repo, number)
	if err != nil {
		return Result{}, fmt.Errorf("get pull request: %w", err)
	}

	if !pr.IsMerged {
		if o.log != nil {
			o.log.Info("skipping cherry-pick: PR not merged", "owner", owner, "repo", repo, "number", number)
		}
		return Result{Skipped: true, SkippedReason: "not merged"}, nil
	}

	if pr.IsFromFork {
		reason := "pull request originates from a fork; create a branch in the base repository before labeling"
		if o.log != nil {
			o.log.Info("skipping cherry-pick: forked pull request", "owner", owner, "repo", repo, "number", number, "head_owner", pr.HeadOwner, "head_repo", pr.HeadRepo)
		}
		return Result{Skipped: true, SkippedReason: reason}, nil
	}

	targets, err := labels.CollectTargets(pr.Labels, o.cfg.LabelPrefix)
	if err != nil {
		return Result{}, fmt.Errorf("collect targets: %w", err)
	}

	targets = o.applyManualTargets(targets)

	if len(targets) == 0 {
		if o.log != nil {
			o.log.Info("skipping cherry-pick: no matching labels or overrides", "owner", owner, "repo", repo, "number", number)
		}
		return Result{Skipped: true, SkippedReason: "no targets"}, nil
	}

	if refreshed, err := o.gh.GetPullRequest(ctx, owner, repo, number); err != nil {
		return Result{}, fmt.Errorf("refresh pull request: %w", err)
	} else {
		pr = refreshed
		targets, err = labels.CollectTargets(pr.Labels, o.cfg.LabelPrefix)
		if err != nil {
			return Result{}, fmt.Errorf("collect targets after refresh: %w", err)
		}
		targets = o.applyManualTargets(targets)
		if len(targets) == 0 {
			if o.log != nil {
				o.log.Info("skipping cherry-pick: labels removed before execution and no overrides remaining", "owner", owner, "repo", repo, "number", number)
			}
			return Result{Skipped: true, SkippedReason: "no targets"}, nil
		}
	}

	if err := labels.ValidateTargets(targets); err != nil {
		return Result{}, fmt.Errorf("validate targets: %w", err)
	}

	sourceCommit, plan, err := o.evaluateTargets(ctx, owner, repo, pr, targets)
	if err != nil {
		return Result{}, err
	}

	if !hasPendingTargets(plan) {
		return Result{Targets: plan}, nil
	}

	if o.cfg.DryRun {
		for i := range plan {
			if plan[i].Status == TargetStatusPending {
				plan[i].Status = TargetStatusDryRun
				plan[i].Reason = "dry run enabled"
			}
		}
		return Result{Targets: plan}, nil
	}

	if o.git == nil {
		return Result{}, fmt.Errorf("git executor is required")
	}

	plan = o.executePendingTargets(ctx, owner, repo, pr, sourceCommit, plan)

	return Result{Targets: plan}, nil
}

func (o *Orchestrator) evaluateTargets(ctx context.Context, owner, repo string, pr gh.PRMetadata, targets []labels.Target) (string, []TargetResult, error) {
	results := make([]TargetResult, 0, len(targets))

	sourceCommit := pr.MergeSHA
	if sourceCommit == "" {
		sourceCommit = pr.HeadSHA
	}

	if sourceCommit == "" {
		return "", nil, fmt.Errorf("source commit SHA could not be determined")
	}

	for _, target := range targets {
		status := TargetResult{Target: target, Status: TargetStatusPending}

		// Check if cherry-pick was already completed for this target (idempotency via done label)
		doneLabel := fmt.Sprintf("%sdone/%s", o.cfg.LabelPrefix, target.Branch)
		hasLabel, err := o.gh.HasLabel(ctx, owner, repo, pr.Number, doneLabel)
		if err != nil {
			// Log warning but continue - don't fail the entire run due to label check failure
			if o.log != nil {
				o.log.Warn("failed to check for done label, continuing anyway", "label", doneLabel, "error", err)
			}
		} else if hasLabel {
			status.Status = TargetStatusSkippedExistingPR
			status.Reason = fmt.Sprintf("already cherry-picked (found %s label)", doneLabel)
			if o.log != nil {
				o.log.Info("skipping cherry-pick target: already completed", "owner", owner, "repo", repo, "target", target.Branch, "label", doneLabel)
			}
			results = append(results, status)
			continue
		}

		if err := o.gh.EnsureBranchExists(ctx, owner, repo, target.Branch); err != nil {
			if err == gh.ErrBranchNotFound {
				status.Status = TargetStatusSkippedNoBranch
				status.Reason = "target branch not found in repository; ensure the release branch exists or remove the label"
				if o.log != nil {
					o.log.Warn("skipping cherry-pick target: branch missing", "owner", owner, "repo", repo, "target", target.Branch)
				}
				results = append(results, status)
				continue
			}
			return "", nil, fmt.Errorf("ensure branch %s: %w", target.Branch, err)
		}

		existing, err := o.gh.ListCherryPickPRs(ctx, owner, repo, pr.Number, target.Branch)
		if err != nil {
			return "", nil, fmt.Errorf("list cherry-pick prs for %s: %w", target.Branch, err)
		}

		if len(existing) > 0 {
			status.Status = TargetStatusSkippedExistingPR
			status.Reason = "cherry-pick PR already exists"
			status.ExistingPR = &existing[0]
			if o.log != nil {
				o.log.Info("skipping cherry-pick target: PR already exists", "owner", owner, "repo", repo, "target", target.Branch, "existing_pr", existing[0].URL)
			}
			results = append(results, status)
			continue
		}

		exists, err := o.gh.CommitExistsOnBranch(ctx, owner, repo, sourceCommit, target.Branch)
		if err != nil {
			return "", nil, fmt.Errorf("check commit on %s: %w", target.Branch, err)
		}

		if exists {
			status.Status = TargetStatusSkippedAlreadyPresent
			status.Reason = "commit already present on target"
			if o.log != nil {
				o.log.Info("skipping cherry-pick target: commit already present", "owner", owner, "repo", repo, "target", target.Branch, "commit", sourceCommit)
			}
			results = append(results, status)
			continue
		}

		results = append(results, status)
	}

	return sourceCommit, results, nil
}

func hasPendingTargets(results []TargetResult) bool {
	for _, res := range results {
		if res.Status == TargetStatusPending {
			return true
		}
	}
	return false
}

func (o *Orchestrator) executePendingTargets(ctx context.Context, owner, repo string, pr gh.PRMetadata, sourceCommit string, plan []TargetResult) []TargetResult {
	updated := make([]TargetResult, len(plan))
	for i, res := range plan {
		if res.Status != TargetStatusPending {
			updated[i] = res
			continue
		}

		updated[i] = o.executeTarget(ctx, owner, repo, pr, sourceCommit, res)
	}
	return updated
}

func (o *Orchestrator) applyManualTargets(labelTargets []labels.Target) []labels.Target {
	if len(o.cfg.TargetBranches) == 0 {
		return labelTargets
	}

	manual := make([]labels.Target, 0, len(o.cfg.TargetBranches))
	for _, branch := range o.cfg.TargetBranches {
		trimmed := strings.TrimSpace(branch)
		if trimmed == "" {
			continue
		}

		normalized := labels.NormalizeBranch(trimmed)
		if normalized == "" {
			continue
		}
		manual = append(manual, labels.Target{
			LabelName: fmt.Sprintf("input:%s", trimmed),
			Branch:    normalized,
		})
	}

	if len(manual) == 0 {
		return labelTargets
	}

	return labels.MergeTargets(labelTargets, manual)
}

func (o *Orchestrator) executeTarget(ctx context.Context, owner, repo string, pr gh.PRMetadata, sourceCommit string, target TargetResult) TargetResult {
	branchName := gh.BranchNameForCherryPick(target.Target.Branch, pr.Number)

	workspace, err := o.git.Prepare(ctx, owner, repo)
	if err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("prepare workspace: %v", err)
		return target
	}

	defer func() {
		if err := workspace.Cleanup(ctx); err != nil && o.log != nil {
			o.log.Warn("failed to cleanup workspace", "error", err, "branch", target.Target.Branch)
		}
	}()

	if err := workspace.CheckoutBranch(ctx, target.Target.Branch); err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("checkout target branch %s: %v", target.Target.Branch, err)
		return target
	}

	if err := workspace.CreateBranchFrom(ctx, branchName, target.Target.Branch); err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("create branch %s: %v", branchName, err)
		return target
	}

	if err := workspace.CheckoutBranch(ctx, branchName); err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("checkout cherry-pick branch %s: %v", branchName, err)
		return target
	}

	if err := workspace.CherryPick(ctx, sourceCommit); err != nil {
		if abortErr := workspace.AbortCherryPick(ctx); abortErr != nil && o.log != nil {
			o.log.Warn("failed to abort cherry-pick after error", "abort_error", abortErr, "target", target.Target.Branch)
		}
		return o.handleCherryPickError(ctx, owner, repo, pr, workspace, branchName, sourceCommit, target, err)
	}

	return o.finalizeCherryPickSuccess(ctx, owner, repo, pr, workspace, branchName, target)
}

func (o *Orchestrator) finalizeCherryPickSuccess(ctx context.Context, owner, repo string, pr gh.PRMetadata, workspace git.Workspace, branchName string, target TargetResult) TargetResult {
	if err := workspace.PushBranch(ctx, branchName); err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("push cherry-pick branch %s: %v", branchName, err)
		return target
	}

	prInput := o.buildCreatePROptions(pr, target.Target, branchName)
	createdPR, err := o.gh.CreatePullRequest(ctx, owner, repo, prInput)
	if err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("create pull request: %v", err)
		return target
	}

	target.Status = TargetStatusSucceeded
	target.Reason = "cherry-pick pull request created"
	target.CreatedPR = &createdPR

	if o.log != nil {
		o.log.Info("created cherry-pick pull request", "owner", owner, "repo", repo, "base_branch", target.Target.Branch, "head_branch", branchName, "pr_number", createdPR.Number, "pr_url", createdPR.URL)
	}

	// Add done label to source PR for idempotency
	doneLabel := fmt.Sprintf("%sdone/%s", o.cfg.LabelPrefix, target.Target.Branch)
	if err := o.gh.AddLabel(ctx, owner, repo, pr.Number, doneLabel); err != nil {
		// Log warning but don't fail - the PR was already created successfully
		if o.log != nil {
			o.log.Warn("failed to add done label to source PR", "label", doneLabel, "source_pr", pr.Number, "error", err)
		}
	} else if o.log != nil {
		o.log.Info("added done label to source PR", "label", doneLabel, "source_pr", pr.Number)
	}

	return target
}

func (o *Orchestrator) handleCherryPickError(ctx context.Context, owner, repo string, pr gh.PRMetadata, workspace git.Workspace, branchName, sourceCommit string, target TargetResult, cherryErr error) TargetResult {
	if o.cfg.ConflictStrategy == conflictStrategyPlaceholderPR {
		return o.handlePlaceholderConflict(ctx, owner, repo, pr, workspace, branchName, target, cherryErr)
	}

	target.Status = TargetStatusFailed
	target.Reason = fmt.Sprintf("cherry-pick commit %s: %v", sourceCommit, cherryErr)
	return target
}

func (o *Orchestrator) handlePlaceholderConflict(ctx context.Context, owner, repo string, pr gh.PRMetadata, workspace git.Workspace, branchName string, target TargetResult, cherryErr error) TargetResult {
	commitMessage := fmt.Sprintf("Placeholder cherry-pick for #%d into %s", pr.Number, target.Target.Branch)
	if err := workspace.CommitAllowEmpty(ctx, commitMessage); err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("placeholder commit failed after conflict (%v): %v", cherryErr, err)
		return target
	}

	if err := workspace.PushBranch(ctx, branchName); err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("push placeholder branch %s failed after conflict (%v): %v", branchName, cherryErr, err)
		return target
	}

	prInput := o.buildCreatePROptions(pr, target.Target, branchName)
	prInput.Body = o.decoratePlaceholderBody(prInput.Body, pr.Number, target.Target.Branch, cherryErr)
	createdPR, err := o.gh.CreatePullRequest(ctx, owner, repo, prInput)
	if err != nil {
		target.Status = TargetStatusFailed
		target.Reason = fmt.Sprintf("create placeholder pull request failed (%v): %v", cherryErr, err)
		return target
	}

	target.Status = TargetStatusPlaceholderPR
	target.Reason = fmt.Sprintf("cherry-pick conflict: placeholder PR opened (%v)", cherryErr)
	target.CreatedPR = &createdPR

	if o.log != nil {
		o.log.Warn("created placeholder cherry-pick pull request", "owner", owner, "repo", repo, "base_branch", target.Target.Branch, "head_branch", branchName, "pr_number", createdPR.Number, "pr_url", createdPR.URL, "error", cherryErr)
	}

	// Add done label to source PR for idempotency (even for placeholder PRs)
	doneLabel := fmt.Sprintf("%sdone/%s", o.cfg.LabelPrefix, target.Target.Branch)
	if err := o.gh.AddLabel(ctx, owner, repo, pr.Number, doneLabel); err != nil {
		// Log warning but don't fail - the PR was already created successfully
		if o.log != nil {
			o.log.Warn("failed to add done label to source PR", "label", doneLabel, "source_pr", pr.Number, "error", err)
		}
	} else if o.log != nil {
		o.log.Info("added done label to source PR", "label", doneLabel, "source_pr", pr.Number)
	}

	return target
}

func (o *Orchestrator) decoratePlaceholderBody(original string, prNumber int, branch string, cherryErr error) string {
	errMsg := strings.TrimSpace(cherryErr.Error())

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("⚠️ Automated cherry-pick of #%d into `%s` encountered conflicts.\n\n", prNumber, branch))
	builder.WriteString("Please resolve the conflicts manually and update this pull request.\n\n")
	if errMsg != "" {
		builder.WriteString("The git command reported:\n\n```\n")
		builder.WriteString(errMsg)
		builder.WriteString("\n```\n\n")
	}
	builder.WriteString(original)
	return builder.String()
}

func (o *Orchestrator) buildCreatePROptions(pr gh.PRMetadata, target labels.Target, branchName string) gh.CreatePROptions {
	title := fmt.Sprintf("[%s] %s", target.Branch, pr.Title)

	var bodyBuilder strings.Builder
	bodyBuilder.WriteString(fmt.Sprintf("%s\n", buildMetadataComment(pr, target)))
	bodyBuilder.WriteString(fmt.Sprintf("Cherry pick of #%d into `%s`.\n\n", pr.Number, target.Branch))
	if pr.Body != "" {
		bodyBuilder.WriteString(pr.Body)
		bodyBuilder.WriteString("\n\n")
	}
	bodyBuilder.WriteString("--\n")
	bodyBuilder.WriteString("Automated cherry-pick by rancher/cherry-pick-action.")

	labels := filterCherryPickLabels(pr.Labels, o.cfg.LabelPrefix)

	return gh.CreatePROptions{
		Title:               title,
		Body:                bodyBuilder.String(),
		Head:                branchName,
		Base:                target.Branch,
		Draft:               false,
		Labels:              labels,
		Assignees:           pr.Assignees,
		MaintainerCanModify: true,
	}
}

func filterCherryPickLabels(all []string, prefix string) []string {
	if len(all) == 0 {
		return nil
	}

	trimmedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	labels := make([]string, 0, len(all))

	for _, label := range all {
		if trimmedPrefix != "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), trimmedPrefix) {
			continue
		}
		labels = append(labels, label)
	}

	return labels
}

func buildMetadataComment(pr gh.PRMetadata, target labels.Target) string {
	owner := strings.TrimSpace(pr.Owner)
	repo := strings.TrimSpace(pr.Repo)
	source := strings.TrimSpace(repo)
	if owner != "" && repo != "" {
		source = fmt.Sprintf("%s/%s", owner, repo)
	} else if source == "" {
		source = "unknown-repo"
	}

	return fmt.Sprintf("<!-- cherry-pick-of: %s#%d -> %s -->", source, pr.Number, target.Branch)
}
