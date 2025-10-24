package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/cherry-pick-action/internal/git"
	gh "github.com/rancher/cherry-pick-action/internal/github"
)

type smokeFactory struct {
	client *smokeClient
}

func (f *smokeFactory) New(ctx context.Context, token string) (gh.Client, error) {
	return f.client, nil
}

type smokeClient struct {
	pr            gh.PRMetadata
	comments      []gh.IssueComment
	createdBodies []string
	updatedBodies map[int64]string
}

func (c *smokeClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (gh.PRMetadata, error) {
	return c.pr, nil
}

func (c *smokeClient) ListCherryPickPRs(ctx context.Context, owner, repo string, sourcePR int, targetBranch string) ([]gh.CherryPickPR, error) {
	return nil, nil
}

func (c *smokeClient) EnsureBranchExists(ctx context.Context, owner, repo, branch string) error {
	return nil
}

func (c *smokeClient) CreateBranch(ctx context.Context, owner, repo, branch, fromSHA string) error {
	return nil
}

func (c *smokeClient) CreatePullRequest(ctx context.Context, owner, repo string, input gh.CreatePROptions) (gh.CherryPickPR, error) {
	return gh.CherryPickPR{}, nil
}

func (c *smokeClient) CommentOnPullRequest(ctx context.Context, owner, repo string, number int, body string) error {
	c.createdBodies = append(c.createdBodies, body)
	return nil
}

func (c *smokeClient) ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]gh.IssueComment, error) {
	return c.comments, nil
}

func (c *smokeClient) UpdateComment(ctx context.Context, owner, repo string, commentID int64, body string) error {
	if c.updatedBodies == nil {
		c.updatedBodies = make(map[int64]string)
	}
	c.updatedBodies[commentID] = body
	return nil
}

func (c *smokeClient) CommitExistsOnBranch(ctx context.Context, owner, repo, commitSHA, branch string) (bool, error) {
	return false, nil
}

func (c *smokeClient) HasLabel(ctx context.Context, owner, repo string, number int, label string) (bool, error) {
	return false, nil
}

func (c *smokeClient) AddLabel(ctx context.Context, owner, repo string, number int, label string) error {
	return nil
}

func (c *smokeClient) CheckOrgMembership(ctx context.Context, org, username string) (bool, error) {
	return true, nil
}

func TestRunnerSmokeDryRun(t *testing.T) {
	tmp := t.TempDir()
	eventPath := filepath.Join(tmp, "event.json")
	summaryPath := filepath.Join(tmp, "summary.md")
	outputPath := filepath.Join(tmp, "outputs.txt")

	eventPayload := map[string]any{
		"action": "closed",
		"repository": map[string]any{
			"owner": map[string]any{"login": "rancher"},
			"name":  "repo",
		},
		"pull_request": map[string]any{
			"number":           42,
			"merged":           true,
			"merge_commit_sha": "abc123",
			"head":             map[string]any{"sha": "def456"},
			"labels":           []map[string]any{{"name": "cherry-pick/release/v0.25"}},
			"assignees":        []map[string]any{{"login": "alice"}},
			"title":            "Improve feature",
			"body":             "Original PR body",
		},
	}

	data, err := json.Marshal(eventPayload)
	if err != nil {
		t.Fatalf("marshal event payload: %v", err)
	}
	if err := os.WriteFile(eventPath, data, 0o644); err != nil {
		t.Fatalf("write event file: %v", err)
	}

	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_EVENT_PATH", eventPath)
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
	t.Setenv("GITHUB_OUTPUT", outputPath)

	client := &smokeClient{pr: gh.PRMetadata{
		Owner:     "rancher",
		Repo:      "repo",
		Number:    42,
		Title:     "Improve feature",
		Body:      "Original PR body",
		MergeSHA:  "abc123",
		HeadSHA:   "def456",
		Labels:    []string{"cherry-pick/release/v0.25"},
		Assignees: []string{"alice"},
		IsMerged:  true,
	}}

	cfg := Config{
		GitHubToken:      "token",
		LabelPrefix:      "cherry-pick/",
		ConflictStrategy: "fail",
		DryRun:           true,
		TargetBranches:   nil,
	}

	runner := NewRunnerWithDeps(cfg, nil, &smokeFactory{client: client}, git.NewNoopExecutor())

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("runner.Run returned error: %v", err)
	}

	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	summary := string(summaryData)
	if !strings.Contains(summary, "release/v0.25") {
		t.Fatalf("expected summary to mention branch, got: %s", summary)
	}
	if !strings.Contains(summary, "dry_run") {
		t.Fatalf("expected summary to include dry_run status, got: %s", summary)
	}

	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read outputs: %v", err)
	}
	outputs := string(outputData)
	if !strings.Contains(outputs, "created_prs") {
		t.Fatalf("expected created_prs output, got: %s", outputs)
	}
	if !strings.Contains(outputs, "skipped_targets") {
		t.Fatalf("expected skipped_targets output, got: %s", outputs)
	}

	if len(client.createdBodies) != 1 {
		t.Fatalf("expected one summary comment, got %d", len(client.createdBodies))
	}
	if !strings.Contains(client.createdBodies[0], "rancher/cherry-pick-action") {
		t.Fatalf("expected comment to include attribution, got: %s", client.createdBodies[0])
	}
	if !strings.Contains(client.createdBodies[0], "release/v0.25") {
		t.Fatalf("expected comment to mention branch, got: %s", client.createdBodies[0])
	}
}
