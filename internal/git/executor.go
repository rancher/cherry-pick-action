package git

import "context"

// Executor manages repository worktrees used for cherry-pick operations.
type Executor interface {
	Prepare(ctx context.Context, owner, repo string) (Workspace, error)
}

// Workspace exposes git primitives required by the orchestrator. Implementations
// may shell out to git or use a pure Go library.
type Workspace interface {
	CheckoutBranch(ctx context.Context, branch string) error
	CreateBranchFrom(ctx context.Context, branch, from string) error
	CherryPick(ctx context.Context, commit string) error
	AbortCherryPick(ctx context.Context) error
	CommitAllowEmpty(ctx context.Context, message string) error
	PushBranch(ctx context.Context, branch string) error
	Cleanup(ctx context.Context) error
}
