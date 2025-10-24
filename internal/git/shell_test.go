package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	gh "github.com/rancher/cherry-pick-action/internal/github"
)

func TestShellExecutorWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tmp := t.TempDir()
	seedRepo := filepath.Join(tmp, "seed")
	remoteRepo := filepath.Join(tmp, "remote.git")

	mustRunGit(t, seedRepo, "init")
	mustRunGit(t, seedRepo, "config", "user.name", "Test User")
	mustRunGit(t, seedRepo, "config", "user.email", "test@example.com")

	writeFile(t, filepath.Join(seedRepo, "README.md"), "initial\n")
	mustRunGit(t, seedRepo, "add", "README.md")
	mustRunGit(t, seedRepo, "commit", "-m", "initial commit")
	mustRunGit(t, seedRepo, "branch", "-M", "main")

	writeFile(t, filepath.Join(seedRepo, "release.txt"), "release\n")
	mustRunGit(t, seedRepo, "checkout", "-b", "release/v1")
	mustRunGit(t, seedRepo, "add", "release.txt")
	mustRunGit(t, seedRepo, "commit", "-m", "release setup")
	mustRunGit(t, seedRepo, "checkout", "main")

	writeFile(t, filepath.Join(seedRepo, "feature.txt"), "feature 1\n")
	mustRunGit(t, seedRepo, "add", "feature.txt")
	mustRunGit(t, seedRepo, "commit", "-m", "feature commit")
	featureSHA := strings.TrimSpace(string(mustCaptureGit(t, seedRepo, "rev-parse", "HEAD")))

	mustRunGit(t, tmp, "init", "--bare", remoteRepo)
	mustRunGit(t, seedRepo, "remote", "add", "origin", remoteRepo)
	mustRunGit(t, seedRepo, "push", "-u", "origin", "main")
	mustRunGit(t, seedRepo, "push", "origin", "release/v1")

	exec := &ShellExecutor{
		BaseDir: filepath.Join(tmp, "workspaces"),
		RemoteURL: func(owner, repo string) string {
			return remoteRepo
		},
		UserName:  "Cherry Pick Bot",
		UserEmail: "bot@example.com",
	}

	workspace, err := exec.Prepare(ctx, "rancher", "repo")
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	defer func() {
		if err := workspace.Cleanup(context.Background()); err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	if err := workspace.CheckoutBranch(ctx, "release/v1"); err != nil {
		t.Fatalf("CheckoutBranch release/v1 failed: %v", err)
	}

	branchName := gh.BranchNameForCherryPick("release/v1", 123)

	if err := workspace.CreateBranchFrom(ctx, branchName, "release/v1"); err != nil {
		t.Fatalf("CreateBranchFrom failed: %v", err)
	}

	if err := workspace.CheckoutBranch(ctx, branchName); err != nil {
		t.Fatalf("CheckoutBranch for new branch failed: %v", err)
	}

	if err := workspace.CherryPick(ctx, featureSHA); err != nil {
		t.Fatalf("CherryPick failed: %v", err)
	}

	if err := workspace.AbortCherryPick(ctx); err != nil {
		t.Fatalf("AbortCherryPick after success should be ignored: %v", err)
	}

	if err := workspace.PushBranch(ctx, branchName); err != nil {
		t.Fatalf("PushBranch failed: %v", err)
	}

	// Ensure branch exists on remote.
	mustCaptureGit(t, "", "--git-dir", remoteRepo, "rev-parse", "--verify", fmt.Sprintf("refs/heads/%s", branchName))
}

func TestShellExecutorMergeCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tmp := t.TempDir()
	seedRepo := filepath.Join(tmp, "seed")
	remoteRepo := filepath.Join(tmp, "remote.git")

	// Initialize seed repo
	mustRunGit(t, seedRepo, "init")
	mustRunGit(t, seedRepo, "config", "user.name", "Test User")
	mustRunGit(t, seedRepo, "config", "user.email", "test@example.com")

	// Create initial commit on main
	writeFile(t, filepath.Join(seedRepo, "README.md"), "initial\n")
	mustRunGit(t, seedRepo, "add", "README.md")
	mustRunGit(t, seedRepo, "commit", "-m", "initial commit")
	mustRunGit(t, seedRepo, "branch", "-M", "main")

	// Create release branch
	writeFile(t, filepath.Join(seedRepo, "release.txt"), "release\n")
	mustRunGit(t, seedRepo, "checkout", "-b", "release/v1")
	mustRunGit(t, seedRepo, "add", "release.txt")
	mustRunGit(t, seedRepo, "commit", "-m", "release setup")
	mustRunGit(t, seedRepo, "checkout", "main")

	// Create a feature branch
	mustRunGit(t, seedRepo, "checkout", "-b", "feature-branch")
	writeFile(t, filepath.Join(seedRepo, "feature1.txt"), "feature 1\n")
	mustRunGit(t, seedRepo, "add", "feature1.txt")
	mustRunGit(t, seedRepo, "commit", "-m", "add feature 1")
	writeFile(t, filepath.Join(seedRepo, "feature2.txt"), "feature 2\n")
	mustRunGit(t, seedRepo, "add", "feature2.txt")
	mustRunGit(t, seedRepo, "commit", "-m", "add feature 2")

	// Merge feature branch into main (creates merge commit)
	mustRunGit(t, seedRepo, "checkout", "main")
	mustRunGit(t, seedRepo, "merge", "--no-ff", "feature-branch", "-m", "Merge feature branch")
	mergeSHA := strings.TrimSpace(string(mustCaptureGit(t, seedRepo, "rev-parse", "HEAD")))

	// Set up remote and push
	mustRunGit(t, tmp, "init", "--bare", remoteRepo)
	mustRunGit(t, seedRepo, "remote", "add", "origin", remoteRepo)
	mustRunGit(t, seedRepo, "push", "-u", "origin", "main")
	mustRunGit(t, seedRepo, "push", "origin", "release/v1")

	exec := &ShellExecutor{
		BaseDir: filepath.Join(tmp, "workspaces"),
		RemoteURL: func(owner, repo string) string {
			return remoteRepo
		},
		UserName:  "Cherry Pick Bot",
		UserEmail: "bot@example.com",
	}

	workspace, err := exec.Prepare(ctx, "rancher", "repo")
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	defer func() {
		if err := workspace.Cleanup(context.Background()); err != nil {
			t.Logf("Cleanup failed: %v", err)
		}
	}()

	if err := workspace.CheckoutBranch(ctx, "release/v1"); err != nil {
		t.Fatalf("CheckoutBranch release/v1 failed: %v", err)
	}

	branchName := gh.BranchNameForCherryPick("release/v1", 456)

	if err := workspace.CreateBranchFrom(ctx, branchName, "release/v1"); err != nil {
		t.Fatalf("CreateBranchFrom failed: %v", err)
	}

	if err := workspace.CheckoutBranch(ctx, branchName); err != nil {
		t.Fatalf("CheckoutBranch for new branch failed: %v", err)
	}

	// This should succeed - cherry-picking a merge commit with -m 1
	if err := workspace.CherryPick(ctx, mergeSHA); err != nil {
		t.Fatalf("CherryPick merge commit failed: %v", err)
	}

	if err := workspace.PushBranch(ctx, branchName); err != nil {
		t.Fatalf("PushBranch failed: %v", err)
	}
}

func TestShellExecutorRetriesNetworkOperations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}

	ctx := context.Background()
	tmp := t.TempDir()
	stateFile := filepath.Join(tmp, "state")
	scriptPath := filepath.Join(tmp, "fakegit.sh")

	script := fmt.Sprintf(`#!/bin/sh
set -e
STATE_FILE=%q
count=0
if [ -f "$STATE_FILE" ]; then
	count=$(cat "$STATE_FILE")
fi
count=$((count + 1))
echo "$count" > "$STATE_FILE"

cmd="$1"
if [ "$cmd" = "-C" ]; then
	shift 2
	cmd="$1"
fi
if [ "$cmd" = "--" ]; then
	shift
	cmd="$1"
fi

if [ "$cmd" = "fetch" ] || [ "$cmd" = "clone" ] || [ "$cmd" = "push" ]; then
	if [ "$count" -lt 3 ]; then
		echo "simulated transient failure" >&2
		exit 128
	fi
fi

exit 0
`, stateFile)

	writeFile(t, scriptPath, script)
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		t.Fatalf("chmod script failed: %v", err)
	}

	exec := &ShellExecutor{
		Git:               scriptPath,
		NetworkRetries:    2,
		NetworkRetryDelay: 10 * time.Millisecond,
		NetworkTimeout:    2 * time.Second,
	}

	if err := exec.runGit(ctx, "-C", tmp, "fetch", "origin", "main"); err != nil {
		attempts := "unknown"
		if data, readErr := os.ReadFile(stateFile); readErr == nil {
			attempts = strings.TrimSpace(string(data))
		}
		t.Fatalf("runGit with retries failed after %s attempts: %v", attempts, err)
	}

	attempts := strings.TrimSpace(readFile(t, stateFile))
	if attempts != "3" {
		t.Fatalf("expected 3 attempts, got %s", attempts)
	}
}

func TestShellExecutorNetworkTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}

	ctx := context.Background()
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "slowgit.sh")

	script := "#!/bin/sh\nsleep 2\nexit 0\n"
	writeFile(t, scriptPath, script)
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		t.Fatalf("chmod script failed: %v", err)
	}

	exec := &ShellExecutor{
		Git:               scriptPath,
		NetworkRetries:    -1, // Explicitly disable retries (0 means default of 2)
		NetworkRetryDelay: 5 * time.Millisecond,
		NetworkTimeout:    100 * time.Millisecond,
	}

	start := time.Now()
	err := exec.runGit(ctx, "fetch", "origin", "main")
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	elapsed := time.Since(start)
	// Allow more margin for CI environments - timeout should happen around 100ms
	// but with context overhead and scheduling, allow up to 300ms
	if elapsed > 300*time.Millisecond {
		t.Fatalf("expected timeout within 300ms, got %v", elapsed)
	}
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
	}
	cmdArgs := append([]string{"-C", dir}, args...)
	if dir == "" {
		cmdArgs = args
	}
	cmd := exec.Command("git", cmdArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(cmdArgs, " "), err, string(output))
	}
}

func mustCaptureGit(t *testing.T, dir string, args ...string) []byte {
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
	}
	cmdArgs := append([]string{"-C", dir}, args...)
	if dir == "" {
		cmdArgs = args
	}
	cmd := exec.Command("git", cmdArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(cmdArgs, " "), err, string(output))
	}
	return output
}

func writeFile(t *testing.T, path, contents string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	return string(data)
}
