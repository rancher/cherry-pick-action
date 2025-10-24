package app

import (
	"context"
	"strings"
	"testing"

	gh "github.com/rancher/cherry-pick-action/internal/github"
	"github.com/rancher/cherry-pick-action/internal/labels"
	"github.com/rancher/cherry-pick-action/internal/orchestrator"
)

type fakeCommentClient struct {
	comments []gh.IssueComment
	created  []string
	updated  map[int64]string
}

func (f *fakeCommentClient) GetPullRequest(context.Context, string, string, int) (gh.PRMetadata, error) {
	panic("not implemented")
}

func (f *fakeCommentClient) ListCherryPickPRs(context.Context, string, string, int, string) ([]gh.CherryPickPR, error) {
	panic("not implemented")
}

func (f *fakeCommentClient) EnsureBranchExists(context.Context, string, string, string) error {
	panic("not implemented")
}

func (f *fakeCommentClient) CreateBranch(context.Context, string, string, string, string) error {
	panic("not implemented")
}

func (f *fakeCommentClient) CreatePullRequest(context.Context, string, string, gh.CreatePROptions) (gh.CherryPickPR, error) {
	panic("not implemented")
}

func (f *fakeCommentClient) CommentOnPullRequest(_ context.Context, _ string, _ string, _ int, body string) error {
	f.created = append(f.created, body)
	return nil
}

func (f *fakeCommentClient) ListPullRequestComments(_ context.Context, _ string, _ string, _ int) ([]gh.IssueComment, error) {
	return f.comments, nil
}

func (f *fakeCommentClient) UpdateComment(_ context.Context, _ string, _ string, commentID int64, body string) error {
	if f.updated == nil {
		f.updated = make(map[int64]string)
	}
	f.updated[commentID] = body
	return nil
}

func (f *fakeCommentClient) CommitExistsOnBranch(context.Context, string, string, string, string) (bool, error) {
	panic("not implemented")
}

func (f *fakeCommentClient) HasLabel(context.Context, string, string, int, string) (bool, error) {
	panic("not implemented")
}

func (f *fakeCommentClient) AddLabel(context.Context, string, string, int, string) error {
	panic("not implemented")
}

func (f *fakeCommentClient) CheckOrgMembership(context.Context, string, string) (bool, error) {
	panic("not implemented")
}

func TestUpsertSummaryCommentCreatesNew(t *testing.T) {
	r := &Runner{}
	client := &fakeCommentClient{}

	result := orchestrator.Result{
		Targets: []orchestrator.TargetResult{
			{
				Target:    labels.Target{Branch: "release/v0.25"},
				Status:    orchestrator.TargetStatusSucceeded,
				Reason:    "created",
				CreatedPR: &gh.CherryPickPR{Number: 101, URL: "https://example.com/pr/101"},
			},
		},
	}

	if err := r.upsertSummaryComment(context.Background(), client, "rancher", "repo", 1, result); err != nil {
		t.Fatalf("upsertSummaryComment returned error: %v", err)
	}

	if len(client.created) != 1 {
		t.Fatalf("expected one created comment, got %d", len(client.created))
	}
	if client.updated != nil {
		t.Fatalf("expected no updates, got %+v", client.updated)
	}
	if got := client.created[0]; !containsMarker(got) {
		t.Fatalf("expected marker in comment body, got %s", got)
	}
}

func TestUpsertSummaryCommentUpdatesExisting(t *testing.T) {
	r := &Runner{}
	existing := buildSummaryCommentBody(orchestrator.Result{Skipped: true, SkippedReason: "previous"})
	client := &fakeCommentClient{
		comments: []gh.IssueComment{{ID: 42, Body: existing + "extra"}},
	}

	result := orchestrator.Result{Skipped: true, SkippedReason: "updated"}

	if err := r.upsertSummaryComment(context.Background(), client, "rancher", "repo", 2, result); err != nil {
		t.Fatalf("upsertSummaryComment returned error: %v", err)
	}

	if len(client.created) != 0 {
		t.Fatalf("expected no new comments, got %d", len(client.created))
	}
	if client.updated == nil || client.updated[42] == "" {
		t.Fatalf("expected comment 42 to be updated, got %+v", client.updated)
	}
	if !containsMarker(client.updated[42]) {
		t.Fatalf("expected marker in updated comment body, got %s", client.updated[42])
	}
}

func TestUpsertSummaryCommentNoChange(t *testing.T) {
	r := &Runner{}
	body := buildSummaryCommentBody(orchestrator.Result{Skipped: true, SkippedReason: "no targets"})
	client := &fakeCommentClient{
		comments: []gh.IssueComment{{ID: 7, Body: body}},
	}

	if err := r.upsertSummaryComment(context.Background(), client, "rancher", "repo", 3, orchestrator.Result{Skipped: true, SkippedReason: "no targets"}); err != nil {
		t.Fatalf("upsertSummaryComment returned error: %v", err)
	}

	if len(client.created) != 0 {
		t.Fatalf("expected no new comments, got %d", len(client.created))
	}
	if client.updated != nil {
		t.Fatalf("expected no updates, got %+v", client.updated)
	}
}

func containsMarker(body string) bool {
	return strings.Contains(body, summaryCommentMarker)
}
