package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/rancher/cherry-pick-action/internal/event"
	"github.com/rancher/cherry-pick-action/internal/git"
	gh "github.com/rancher/cherry-pick-action/internal/github"
	"github.com/rancher/cherry-pick-action/internal/orchestrator"
)

// Runner glues together the orchestrator and supporting services to execute the cherry-pick flow.
type Runner struct {
	cfg       Config
	log       *slog.Logger
	ghFactory gh.Factory
	gitExec   git.Executor // only set for testing via NewRunnerWithDeps
}

// NewRunner constructs a Runner with the supplied configuration.
func NewRunner(cfg Config) (*Runner, error) {
	logger, err := NewLogger(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return nil, fmt.Errorf("create logger: %w", err)
	}

	return &Runner{
		cfg:       cfg,
		log:       logger,
		ghFactory: gh.NewRESTFactory(cfg.GitHubBaseURL, cfg.GitHubUploadURL),
		gitExec:   nil,
	}, nil
}

// NewRunnerWithDeps constructs a Runner with injected dependencies for testing.
func NewRunnerWithDeps(cfg Config, log *slog.Logger, ghFactory gh.Factory, gitExec git.Executor) *Runner {
	return &Runner{cfg: cfg, log: log, ghFactory: ghFactory, gitExec: gitExec}
}

// Run executes the application using the provided context.
func (r *Runner) Run(ctx context.Context) error {
	if r.log != nil {
		r.log.Info("starting cherry-pick action run", "dry_run", r.cfg.DryRun, "conflict_strategy", r.cfg.ConflictStrategy)
	}

	eventName := strings.TrimSpace(os.Getenv("GITHUB_EVENT_NAME"))
	if eventName != "pull_request" && eventName != "pull_request_target" {
		if r.log != nil {
			r.log.Info("ignoring unsupported event", "event_name", eventName)
		}
		return nil
	}

	eventPath := strings.TrimSpace(os.Getenv("GITHUB_EVENT_PATH"))
	if eventPath == "" {
		return fmt.Errorf("GITHUB_EVENT_PATH is required for pull_request events")
	}

	payload, err := event.ParsePullRequestEventFile(eventPath)
	if err != nil {
		return fmt.Errorf("parse pull request event: %w", err)
	}

	if payload.Action != event.PullRequestActionClosed && payload.Action != event.PullRequestActionLabeled {
		if r.log != nil {
			r.log.Info("ignoring unsupported pull_request action", "action", payload.Action)
		}
		return nil
	}

	if payload.Repository.Owner == "" || payload.Repository.Name == "" {
		return fmt.Errorf("event payload missing repository owner/name")
	}

	if payload.PullRequest.Number == 0 {
		return fmt.Errorf("event payload missing pull request number")
	}

	ghClient, err := r.ghFactory.New(ctx, r.cfg.GitHubToken)
	if err != nil {
		return fmt.Errorf("initialize github client: %w", err)
	}

	// Check organization membership if required
	if r.cfg.RequireOrgMembership {
		actor := strings.TrimSpace(os.Getenv("GITHUB_ACTOR"))
		if actor == "" {
			return fmt.Errorf("GITHUB_ACTOR environment variable is required when require_org_membership is enabled")
		}

		isMember, err := ghClient.CheckOrgMembership(ctx, payload.Repository.Owner, actor)
		if err != nil {
			return fmt.Errorf("check organization membership for %q in %q: %w", actor, payload.Repository.Owner, err)
		}

		if !isMember {
			if r.log != nil {
				r.log.Info("skipping cherry-pick: actor is not a member of the repository owner organization",
					"actor", actor,
					"organization", payload.Repository.Owner)
			}
			return nil
		}

		if r.log != nil {
			r.log.Debug("organization membership check passed", "actor", actor, "organization", payload.Repository.Owner)
		}
	}

	gitExec := r.gitExec
	if gitExec == nil && !r.cfg.DryRun {
		exec, err := r.buildGitExecutor()
		if err != nil {
			return fmt.Errorf("configure git executor: %w", err)
		}
		gitExec = exec
	}

	orchCfg := orchestrator.Config{
		LabelPrefix:      r.cfg.LabelPrefix,
		ConflictStrategy: r.cfg.ConflictStrategy,
		DryRun:           r.cfg.DryRun,
		TargetBranches:   r.cfg.TargetBranches,
	}

	orch := orchestrator.New(orchCfg, ghClient, gitExec, r.log)

	result, err := orch.ProcessPullRequest(ctx, payload.Repository.Owner, payload.Repository.Name, payload.PullRequest.Number)
	if err != nil {
		return fmt.Errorf("process pull request: %w", err)
	}

	if result.Skipped {
		if r.log != nil {
			r.log.Info("skipping cherry-pick orchestration", "reason", result.SkippedReason)
		}
		if err := r.writeStepSummary(result); err != nil && r.log != nil {
			r.log.Warn("failed to write step summary", "error", err)
		}
		if err := r.writeGitHubOutputs(result); err != nil && r.log != nil {
			r.log.Warn("failed to write action outputs", "error", err)
		}
		if err := r.upsertSummaryComment(ctx, ghClient, payload.Repository.Owner, payload.Repository.Name, payload.PullRequest.Number, result); err != nil && r.log != nil {
			r.log.Warn("failed to post pull request comment", "error", err)
		}
		return nil
	}

	for _, target := range result.Targets {
		if r.log != nil {
			r.log.Info("evaluated cherry-pick target", "branch", target.Target.Branch, "status", target.Status, "reason", target.Reason)
		}
	}

	if err := r.writeStepSummary(result); err != nil && r.log != nil {
		r.log.Warn("failed to write step summary", "error", err)
	}

	if err := r.writeGitHubOutputs(result); err != nil && r.log != nil {
		r.log.Warn("failed to write action outputs", "error", err)
	}

	if err := r.upsertSummaryComment(ctx, ghClient, payload.Repository.Owner, payload.Repository.Name, payload.PullRequest.Number, result); err != nil && r.log != nil {
		r.log.Warn("failed to post pull request comment", "error", err)
	}

	// Check if any targets failed and return an error to fail the workflow
	var failedTargets []string
	for _, target := range result.Targets {
		if target.Status == "failed" {
			failedTargets = append(failedTargets, target.Target.Branch)
		}
	}

	if len(failedTargets) > 0 {
		return fmt.Errorf("cherry-pick failed for %d target(s): %s", len(failedTargets), strings.Join(failedTargets, ", "))
	}

	return nil
}

func (r *Runner) buildGitExecutor() (git.Executor, error) {
	exec := git.NewShellExecutor()
	exec.Token = r.cfg.GitHubToken
	exec.UserName = r.cfg.GitUserName
	exec.UserEmail = r.cfg.GitUserEmail
	exec.SigningKey = r.cfg.GitSigningKey
	exec.SigningPassphrase = r.cfg.GitSigningPass

	if remote := remoteURLBuilder(r.cfg); remote != nil {
		exec.RemoteURL = remote
	}

	return exec, nil
}

func remoteURLBuilder(cfg Config) func(owner, repo string) string {
	base := strings.TrimSpace(cfg.GitHubBaseURL)
	if base == "" {
		return nil
	}

	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}

	root := (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
	root = strings.TrimRight(root, "/")

	return func(owner, repo string) string {
		return fmt.Sprintf("%s/%s/%s.git", root, owner, repo)
	}
}
