package gh

import (
	"context"
	"errors"
)

// PRMetadata contains source pull request details needed for cherry-pick operations.
type PRMetadata struct {
	Owner      string
	Repo       string
	Number     int
	Title      string
	Body       string
	MergeSHA   string
	HeadSHA    string
	HeadRef    string
	HeadRepo   string
	HeadOwner  string
	Labels     []string
	Assignees  []string
	IsMerged   bool
	IsFromFork bool
}

// CherryPickPR represents a newly created cherry-pick pull request.
type CherryPickPR struct {
	URL    string
	Number int
	Head   string
	Base   string
}

// IssueComment represents a GitHub issue or pull request comment.
type IssueComment struct {
	ID   int64
	Body string
}

// Client exposes the GitHub operations required by the cherry-pick orchestrator.
type Client interface {
	GetPullRequest(ctx context.Context, owner, repo string, number int) (PRMetadata, error)
	ListCherryPickPRs(ctx context.Context, owner, repo string, sourcePR int, targetBranch string) ([]CherryPickPR, error)
	EnsureBranchExists(ctx context.Context, owner, repo, branch string) error
	CreateBranch(ctx context.Context, owner, repo, branch, fromSHA string) error
	CreatePullRequest(ctx context.Context, owner, repo string, input CreatePROptions) (CherryPickPR, error)
	CommentOnPullRequest(ctx context.Context, owner, repo string, number int, body string) error
	ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error)
	UpdateComment(ctx context.Context, owner, repo string, commentID int64, body string) error
	CommitExistsOnBranch(ctx context.Context, owner, repo, commitSHA, branch string) (bool, error)
	HasLabel(ctx context.Context, owner, repo string, number int, label string) (bool, error)
	AddLabel(ctx context.Context, owner, repo string, number int, label string) error
	CheckOrgMembership(ctx context.Context, org, username string) (bool, error)
}

// CreatePROptions defines the metadata required to open a cherry-pick PR.
type CreatePROptions struct {
	Title               string
	Body                string
	Head                string
	Base                string
	Draft               bool
	Labels              []string
	Assignees           []string
	MaintainerCanModify bool
}

// Factory builds concrete GitHub clients (e.g., REST-backed) for the orchestrator.
type Factory interface {
	New(ctx context.Context, token string) (Client, error)
}

// ErrBranchNotFound indicates the requested target branch does not exist.
var ErrBranchNotFound = errors.New("github: branch not found")

// retryableError marks an error that may succeed if the operation is retried.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *retryableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// IsRetryable reports whether the supplied error resulted from a retryable GitHub
// API failure (for example, a transient network problem or rate-limited request).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var target *retryableError
	return errors.As(err, &target)
}
