package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ShellExecutor shells out to the system git binary to prepare workspaces for
// cherry-pick operations.
type ShellExecutor struct {
	// Git is the git binary to execute. Defaults to "git" when empty.
	Git string

	// BaseDir is the directory under which temporary workspaces are created. When
	// empty, os.TempDir() is used.
	BaseDir string

	// RemoteURL constructs the git remote URL for the given owner/repo pair. When
	// unset, https://github.com/<owner>/<repo>.git is assumed.
	RemoteURL func(owner, repo string) string

	// Token, if provided, is embedded into HTTPS remotes using the
	// x-access-token format.
	Token string

	// UserName and UserEmail configure the git identity for commits.
	UserName  string
	UserEmail string

	// SigningKey, if provided, enables GPG signing of commits. The key should be
	// base64-encoded or armored GPG private key material.
	SigningKey string

	// SigningPassphrase unlocks the signing key when required.
	SigningPassphrase string

	// RemoteName controls which remote the workspace interacts with. Defaults to "origin".
	RemoteName string

	// NetworkRetries controls how many additional attempts should be made for network
	// oriented git commands (clone, fetch, push). When zero, a default of 2 retries is used.
	NetworkRetries int

	// NetworkRetryDelay controls the initial backoff delay between retries. When zero,
	// a default of 1 second is used. Backoff grows exponentially per attempt.
	NetworkRetryDelay time.Duration

	// NetworkTimeout bounds network commands that would otherwise inherit an unbounded
	// context. When zero, a default of 2 minutes is used.
	NetworkTimeout time.Duration
}

// NewShellExecutor returns an Executor backed by system git commands.
func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{}
}

func (e *ShellExecutor) gitBinary() string {
	if e.Git == "" {
		return "git"
	}
	return e.Git
}

func (e *ShellExecutor) remoteName() string {
	if e.RemoteName == "" {
		return "origin"
	}
	return e.RemoteName
}

func (e *ShellExecutor) remoteURL(owner, repo string) string {
	if e.RemoteURL != nil {
		return e.RemoteURL(owner, repo)
	}
	url := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	if e.Token == "" {
		return url
	}
	parts := strings.SplitN(strings.TrimPrefix(url, "https://"), "/", 2)
	if len(parts) != 2 {
		return url
	}
	return fmt.Sprintf("https://x-access-token:%s@%s/%s", e.Token, parts[0], parts[1])
}

func (e *ShellExecutor) workspaceDir(repo string) (string, error) {
	base := e.BaseDir
	if base == "" {
		base = os.TempDir()
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("create workspace base: %w", err)
	}
	return os.MkdirTemp(base, fmt.Sprintf("cherry-pick-%s-", strings.ReplaceAll(repo, " ", "_")))
}

func (e *ShellExecutor) Prepare(ctx context.Context, owner, repo string) (Workspace, error) {
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo are required")
	}

	remoteURL := e.remoteURL(owner, repo)
	if remoteURL == "" {
		return nil, fmt.Errorf("remote url could not be determined")
	}

	workDir, err := e.workspaceDir(repo)
	if err != nil {
		return nil, err
	}

	cleanup := func() {
		_ = os.RemoveAll(workDir)
	}

	if err := e.runGit(ctx, "clone", "--filter=blob:none", "--no-checkout", remoteURL, workDir); err != nil {
		if !shouldRetryWithoutFilter(err) {
			cleanup()
			return nil, fmt.Errorf("git clone: %w", err)
		}

		cleanup()

		workDir, err = e.workspaceDir(repo)
		if err != nil {
			return nil, err
		}

		cleanup = func() {
			_ = os.RemoveAll(workDir)
		}

		if err := e.runGit(ctx, "clone", "--no-checkout", remoteURL, workDir); err != nil {
			cleanup()
			return nil, fmt.Errorf("git clone: %w", err)
		}
	}

	if e.UserName != "" {
		if err := e.runGit(ctx, "-C", workDir, "config", "user.name", e.UserName); err != nil {
			cleanup()
			return nil, fmt.Errorf("git config user.name: %w", err)
		}
	}
	if e.UserEmail != "" {
		if err := e.runGit(ctx, "-C", workDir, "config", "user.email", e.UserEmail); err != nil {
			cleanup()
			return nil, fmt.Errorf("git config user.email: %w", err)
		}
	}

	if e.SigningKey != "" {
		if err := e.configureGPGSigning(ctx, workDir); err != nil {
			cleanup()
			return nil, fmt.Errorf("configure gpg signing: %w", err)
		}
	}

	ws := &shellWorkspace{
		executor:   e,
		path:       workDir,
		remoteName: e.remoteName(),
	}

	return ws, nil
}

type shellWorkspace struct {
	path       string
	remoteName string
	executor   *ShellExecutor
}

func (w *shellWorkspace) CheckoutBranch(ctx context.Context, branch string) error {
	ref := fmt.Sprintf("%s/%s", w.remoteName, branch)
	fetchErr := w.exec(ctx, "fetch", w.remoteName, branch)
	if fetchErr != nil && !isMissingRemoteBranch(fetchErr) {
		return fmt.Errorf("git fetch %s: %w", branch, fetchErr)
	}

	if fetchErr == nil {
		if err := w.exec(ctx, "checkout", "-B", branch, ref); err == nil {
			return nil
		}
	}

	if err := w.exec(ctx, "checkout", branch); err == nil {
		return nil
	}

	if fetchErr == nil {
		if err := w.exec(ctx, "checkout", "-b", branch, ref); err == nil {
			return nil
		}
	}

	return fmt.Errorf("git checkout %s failed", branch)
}

func (w *shellWorkspace) CreateBranchFrom(ctx context.Context, branch, from string) error {
	ref := fmt.Sprintf("%s/%s", w.remoteName, from)
	if err := w.exec(ctx, "fetch", w.remoteName, from); err != nil {
		return fmt.Errorf("git fetch %s: %w", from, err)
	}
	if err := w.exec(ctx, "branch", "--force", branch, ref); err != nil {
		return fmt.Errorf("git branch %s from %s: %w", branch, from, err)
	}
	return nil
}

func (w *shellWorkspace) CherryPick(ctx context.Context, commit string) error {
	// First, check if this is a merge commit
	isMerge, err := w.isMergeCommit(ctx, commit)
	if err != nil {
		return fmt.Errorf("check if merge commit: %w", err)
	}

	// For merge commits, use -m 1 to specify the first parent as mainline
	if isMerge {
		if err := w.exec(ctx, "cherry-pick", "-m", "1", commit); err != nil {
			return fmt.Errorf("git cherry-pick %s: %w", commit, err)
		}
	} else {
		if err := w.exec(ctx, "cherry-pick", commit); err != nil {
			return fmt.Errorf("git cherry-pick %s: %w", commit, err)
		}
	}
	return nil
}

func (w *shellWorkspace) isMergeCommit(ctx context.Context, commit string) (bool, error) {
	// Use git rev-list to count the parents of a commit
	// Merge commits have 2+ parents, regular commits have 1
	output, err := w.executor.captureGitOutput(ctx, "-C", w.path, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return false, err
	}

	// Output format: "commit_sha parent1_sha [parent2_sha ...]"
	// Split by whitespace and count - if more than 2 fields, it's a merge
	fields := strings.Fields(strings.TrimSpace(output))
	return len(fields) > 2, nil
}

func (e *ShellExecutor) captureGitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, e.gitBinary(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", &GitError{Args: args, Output: string(output), Err: err}
	}
	return string(output), nil
}

func (w *shellWorkspace) AbortCherryPick(ctx context.Context) error {
	err := w.exec(ctx, "cherry-pick", "--abort")
	if err == nil {
		return nil
	}
	var gitErr *GitError
	if errors.As(err, &gitErr) {
		if strings.Contains(strings.ToLower(gitErr.Output), "no cherry-pick") {
			return nil
		}
	}
	return err
}

func (w *shellWorkspace) CommitAllowEmpty(ctx context.Context, message string) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "cherry-pick placeholder"
	}
	if err := w.exec(ctx, "commit", "--allow-empty", "-m", msg); err != nil {
		return fmt.Errorf("git commit --allow-empty: %w", err)
	}
	return nil
}

func (w *shellWorkspace) PushBranch(ctx context.Context, branch string) error {
	if err := w.exec(ctx, "push", w.remoteName, "--force-with-lease", fmt.Sprintf("%s:%s", branch, branch)); err != nil {
		return fmt.Errorf("git push %s: %w", branch, err)
	}
	return nil
}

func (w *shellWorkspace) Cleanup(ctx context.Context) error {
	return os.RemoveAll(w.path)
}

func (w *shellWorkspace) exec(ctx context.Context, args ...string) error {
	cmd := append([]string{"-C", w.path}, args...)
	return w.executor.runGit(ctx, cmd...)
}

func (e *ShellExecutor) runGit(ctx context.Context, args ...string) error {
	primary := primaryGitCommand(args)
	isNetwork := isNetworkCommand(primary)

	retries := 0
	if isNetwork {
		retries = e.networkRetriesValue()
	}

	delay := e.networkRetryDelayValue()
	var lastErr error

	for attempt := 0; attempt <= retries; attempt++ {
		attemptCtx, cancel := e.applyNetworkTimeout(ctx, isNetwork)
		err := e.runGitOnce(attemptCtx, args...)
		cancel()

		if err == nil {
			return nil
		}
		lastErr = err

		if !isNetwork {
			break
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if attempt == retries {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < time.Second {
			delay = time.Second
		}
		delay *= 2
	}

	return lastErr
}

func (e *ShellExecutor) runGitOnce(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, e.gitBinary(), args...)
	setProcessGroup(cmd)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return &GitError{Args: args, Output: output.String(), Err: err}
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		terminateProcessGroup(cmd)
		err := <-done
		if err != nil {
			return ctx.Err()
		}
		return ctx.Err()
	case err := <-done:
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return &GitError{Args: args, Output: output.String(), Err: err}
		}
	}

	return nil
}

func primaryGitCommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "-C", "--git-dir", "-c":
				i++
			}
			continue
		}
		return arg
	}
	return ""
}

func isNetworkCommand(cmd string) bool {
	switch cmd {
	case "clone", "fetch", "push", "pull", "remote":
		return true
	default:
		return false
	}
}

func (e *ShellExecutor) networkRetriesValue() int {
	if e.NetworkRetries < 0 {
		return 0
	}
	if e.NetworkRetries == 0 {
		return 2
	}
	return e.NetworkRetries
}

func (e *ShellExecutor) networkRetryDelayValue() time.Duration {
	if e.NetworkRetryDelay <= 0 {
		return time.Second
	}
	return e.NetworkRetryDelay
}

func (e *ShellExecutor) networkTimeoutValue() time.Duration {
	if e.NetworkTimeout <= 0 {
		return 2 * time.Minute
	}
	return e.NetworkTimeout
}

func (e *ShellExecutor) applyNetworkTimeout(ctx context.Context, network bool) (context.Context, context.CancelFunc) {
	if !network {
		return ctx, func() {}
	}
	if deadline, ok := ctx.Deadline(); ok && !deadline.IsZero() {
		return ctx, func() {}
	}
	timeout := e.networkTimeoutValue()
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// GitError wraps failures when invoking the git binary.
type GitError struct {
	Args   []string
	Output string
	Err    error
}

func (e *GitError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("git %s: %v\n%s", strings.Join(e.Args, " "), e.Err, e.Output)
}

func (e *GitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func isMissingRemoteBranch(err error) bool {
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		return false
	}
	out := gitErr.Output
	return strings.Contains(out, "couldn't find remote ref") ||
		strings.Contains(out, "invalid refspec") ||
		strings.Contains(out, "unknown revision")
}

func shouldRetryWithoutFilter(err error) bool {
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		return false
	}

	output := strings.ToLower(gitErr.Output)
	return strings.Contains(output, "filter") || strings.Contains(output, "partial clone")
}

func (e *ShellExecutor) configureGPGSigning(ctx context.Context, workDir string) error {
	keyData := strings.TrimSpace(e.SigningKey)
	if keyData == "" {
		return nil
	}

	// Import the GPG key
	gpgHome := filepath.Join(workDir, ".gnupg")
	if err := os.MkdirAll(gpgHome, 0o700); err != nil {
		return fmt.Errorf("create gpg home: %w", err)
	}

	keyFile := filepath.Join(gpgHome, "signing.key")
	if err := os.WriteFile(keyFile, []byte(keyData), 0o600); err != nil {
		return fmt.Errorf("write signing key: %w", err)
	}
	defer func() {
		if err := os.Remove(keyFile); err != nil {
			// Log but don't fail - cleanup is best effort
			fmt.Fprintf(os.Stderr, "failed to remove temp key file: %v\n", err)
		}
	}()

	gpgCmd := exec.CommandContext(ctx, "gpg", "--homedir", gpgHome, "--batch", "--import", keyFile)
	if e.SigningPassphrase != "" {
		gpgCmd.Env = append(os.Environ(), fmt.Sprintf("GPG_PASSPHRASE=%s", e.SigningPassphrase))
	}

	if output, err := gpgCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gpg import key: %w\n%s", err, string(output))
	}

	// Extract key ID
	listCmd := exec.CommandContext(ctx, "gpg", "--homedir", gpgHome, "--list-secret-keys", "--keyid-format=long")
	output, err := listCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gpg list keys: %w\n%s", err, string(output))
	}

	keyID := extractKeyID(string(output))
	if keyID == "" {
		return fmt.Errorf("could not extract key ID from gpg output")
	}

	// Configure git to use the key
	if err := e.runGit(ctx, "-C", workDir, "config", "user.signingkey", keyID); err != nil {
		return fmt.Errorf("git config user.signingkey: %w", err)
	}

	if err := e.runGit(ctx, "-C", workDir, "config", "commit.gpgsign", "true"); err != nil {
		return fmt.Errorf("git config commit.gpgsign: %w", err)
	}

	if err := e.runGit(ctx, "-C", workDir, "config", "gpg.program", "gpg"); err != nil {
		return fmt.Errorf("git config gpg.program: %w", err)
	}

	return nil
}

func extractKeyID(output string) string {
	// Look for pattern like "rsa4096/ABCD1234EFGH5678"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "sec") || strings.Contains(line, "ssb") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, "/") {
					segments := strings.Split(part, "/")
					if len(segments) == 2 && len(segments[1]) >= 8 {
						return segments[1]
					}
				}
			}
		}
	}
	return ""
}
