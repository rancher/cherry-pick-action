package git

import (
	"context"
	"testing"
)

func TestNoopExecutorPrepare(t *testing.T) {
	ctx := context.Background()
	exec := NewNoopExecutor()

	workspace, err := exec.Prepare(ctx, "rancher", "repo")
	if err != nil {
		t.Fatalf("Prepare returned unexpected error: %v", err)
	}

	if workspace == nil {
		t.Fatalf("expected workspace, got nil")
	}
}

func TestNoopWorkspaceOperations(t *testing.T) {
	ctx := context.Background()
	exec := NewNoopExecutor()

	workspace, err := exec.Prepare(ctx, "rancher", "repo")
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	// All operations should succeed without side effects
	if err := workspace.CheckoutBranch(ctx, "main"); err != nil {
		t.Fatalf("CheckoutBranch failed: %v", err)
	}

	if err := workspace.CreateBranchFrom(ctx, "test-branch", "main"); err != nil {
		t.Fatalf("CreateBranchFrom failed: %v", err)
	}

	if err := workspace.CherryPick(ctx, "abc123"); err != nil {
		t.Fatalf("CherryPick failed: %v", err)
	}

	if err := workspace.AbortCherryPick(ctx); err != nil {
		t.Fatalf("AbortCherryPick failed: %v", err)
	}

	if err := workspace.CommitAllowEmpty(ctx, "test message"); err != nil {
		t.Fatalf("CommitAllowEmpty failed: %v", err)
	}

	if err := workspace.PushBranch(ctx, "test-branch"); err != nil {
		t.Fatalf("PushBranch failed: %v", err)
	}

	if err := workspace.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
}

func TestNoopExecutorMultipleWorkspaces(t *testing.T) {
	ctx := context.Background()
	exec := NewNoopExecutor()

	// Should be able to create multiple workspaces
	ws1, err := exec.Prepare(ctx, "rancher", "repo1")
	if err != nil {
		t.Fatalf("Prepare workspace 1 failed: %v", err)
	}

	ws2, err := exec.Prepare(ctx, "rancher", "repo2")
	if err != nil {
		t.Fatalf("Prepare workspace 2 failed: %v", err)
	}

	// Both workspaces should function independently
	if err := ws1.CheckoutBranch(ctx, "branch1"); err != nil {
		t.Fatalf("workspace 1 operation failed: %v", err)
	}

	if err := ws2.CheckoutBranch(ctx, "branch2"); err != nil {
		t.Fatalf("workspace 2 operation failed: %v", err)
	}

	// Cleanup should succeed for both
	if err := ws1.Cleanup(ctx); err != nil {
		t.Fatalf("workspace 1 cleanup failed: %v", err)
	}

	if err := ws2.Cleanup(ctx); err != nil {
		t.Fatalf("workspace 2 cleanup failed: %v", err)
	}
}
