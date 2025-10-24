package gh

import (
	"context"
	"fmt"
)

// NewNoopFactory returns a Factory that builds noop clients.
func NewNoopFactory() Factory {
	return noopFactory{}
}

type noopFactory struct{}

func (noopFactory) New(ctx context.Context, token string) (Client, error) {
	return noopClient{}, nil
}

type noopClient struct{}

func (noopClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (PRMetadata, error) {
	return PRMetadata{}, fmt.Errorf("noop github client not implemented")
}

func (noopClient) ListCherryPickPRs(ctx context.Context, owner, repo string, sourcePR int, targetBranch string) ([]CherryPickPR, error) {
	return nil, fmt.Errorf("noop github client not implemented")
}

func (noopClient) EnsureBranchExists(ctx context.Context, owner, repo, branch string) error {
	return fmt.Errorf("noop github client not implemented")
}

func (noopClient) CreateBranch(ctx context.Context, owner, repo, branch, fromSHA string) error {
	return fmt.Errorf("noop github client not implemented")
}

func (noopClient) CreatePullRequest(ctx context.Context, owner, repo string, input CreatePROptions) (CherryPickPR, error) {
	return CherryPickPR{}, fmt.Errorf("noop github client not implemented")
}

func (noopClient) CommentOnPullRequest(ctx context.Context, owner, repo string, number int, body string) error {
	return fmt.Errorf("noop github client not implemented")
}

func (noopClient) ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	return nil, fmt.Errorf("noop github client not implemented")
}

func (noopClient) UpdateComment(ctx context.Context, owner, repo string, commentID int64, body string) error {
	return fmt.Errorf("noop github client not implemented")
}

func (noopClient) CommitExistsOnBranch(ctx context.Context, owner, repo, commitSHA, branch string) (bool, error) {
	return false, fmt.Errorf("noop github client not implemented")
}

func (noopClient) HasLabel(ctx context.Context, owner, repo string, number int, label string) (bool, error) {
	return false, fmt.Errorf("noop github client not implemented")
}

func (noopClient) AddLabel(ctx context.Context, owner, repo string, number int, label string) error {
	return fmt.Errorf("noop github client not implemented")
}

func (noopClient) CheckOrgMembership(ctx context.Context, org, username string) (bool, error) {
	return false, fmt.Errorf("noop github client not implemented")
}
