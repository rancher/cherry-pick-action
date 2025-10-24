package git

import (
	"context"
)

// NewNoopExecutor returns an Executor that performs no actual git operations.
// All workspace methods succeed without side effects, useful for testing and dry-run scenarios.
func NewNoopExecutor() Executor {
	return &noopExecutor{}
}

type noopExecutor struct{}

func (e *noopExecutor) Prepare(ctx context.Context, owner, repo string) (Workspace, error) {
	return &noopWorkspace{}, nil
}

type noopWorkspace struct{}

func (w *noopWorkspace) CheckoutBranch(ctx context.Context, branch string) error {
	return nil
}

func (w *noopWorkspace) CreateBranchFrom(ctx context.Context, branch, from string) error {
	return nil
}

func (w *noopWorkspace) CherryPick(ctx context.Context, commit string) error {
	return nil
}

func (w *noopWorkspace) AbortCherryPick(ctx context.Context) error {
	return nil
}

func (w *noopWorkspace) CommitAllowEmpty(ctx context.Context, message string) error {
	return nil
}

func (w *noopWorkspace) PushBranch(ctx context.Context, branch string) error {
	return nil
}

func (w *noopWorkspace) Cleanup(ctx context.Context) error {
	return nil
}
