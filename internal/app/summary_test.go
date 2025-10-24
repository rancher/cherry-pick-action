package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gh "github.com/rancher/cherry-pick-action/internal/github"
	"github.com/rancher/cherry-pick-action/internal/labels"
	"github.com/rancher/cherry-pick-action/internal/orchestrator"
)

func TestWriteStepSummarySkipped(t *testing.T) {
	r := &Runner{}
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	result := orchestrator.Result{Skipped: true, SkippedReason: "no targets"}
	if err := r.writeStepSummary(result); err != nil {
		t.Fatalf("writeStepSummary returned error: %v", err)
	}

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed reading summary: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Cherry-pick action summary") {
		t.Fatalf("expected summary header, got: %s", content)
	}
	if !strings.Contains(content, "no targets") {
		t.Fatalf("expected skip reason in summary, got: %s", content)
	}
}

func TestWriteStepSummaryTargets(t *testing.T) {
	r := &Runner{}
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	result := orchestrator.Result{
		Targets: []orchestrator.TargetResult{
			{
				Target:    labels.Target{Branch: "release/v0.25"},
				Status:    orchestrator.TargetStatusSucceeded,
				Reason:    "cherry-pick pull request created",
				CreatedPR: &gh.CherryPickPR{Number: 101, URL: "https://example.com/pr/101"},
			},
			{
				Target: labels.Target{Branch: "release/v0.24"},
				Status: orchestrator.TargetStatusSkippedNoBranch,
				Reason: "target branch not found",
			},
		},
	}

	if err := r.writeStepSummary(result); err != nil {
		t.Fatalf("writeStepSummary returned error: %v", err)
	}

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed reading summary: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "| Branch | Status | Details | PR |") {
		t.Fatalf("expected table header with PR column, got: %s", content)
	}

	if !strings.Contains(content, "release/v0.25") || !strings.Contains(content, "release/v0.24") {
		t.Fatalf("expected branches in summary, got: %s", content)
	}
	if !strings.Contains(content, "succeeded") || !strings.Contains(content, "skipped_missing_branch") {
		t.Fatalf("expected statuses in summary, got: %s", content)
	}
	if !strings.Contains(content, "PR #101") {
		t.Fatalf("expected created PR reference in summary, got: %s", content)
	}
}

func TestWriteGitHubOutputs(t *testing.T) {
	r := &Runner{}
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "output.tmp")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	result := orchestrator.Result{
		Targets: []orchestrator.TargetResult{
			{
				Target:    labels.Target{Branch: "release/v0.25"},
				Status:    orchestrator.TargetStatusSucceeded,
				Reason:    "created",
				CreatedPR: &gh.CherryPickPR{Number: 101, URL: "https://example.com/pr/101", Head: "cherry-pick/release/v0.25/pr-1", Base: "release/v0.25"},
			},
			{
				Target: labels.Target{Branch: "release/v0.24"},
				Status: orchestrator.TargetStatusSkippedNoBranch,
				Reason: "missing branch",
			},
		},
	}

	if err := r.writeGitHubOutputs(result); err != nil {
		t.Fatalf("writeGitHubOutputs returned error: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed reading output file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "created_prs<<EOF") {
		t.Fatalf("expected created_prs output, got: %s", content)
	}
	if !strings.Contains(content, "\"branch\":\"release/v0.25\"") {
		t.Fatalf("expected created PR JSON payload, got: %s", content)
	}
	if !strings.Contains(content, "skipped_targets<<EOF") {
		t.Fatalf("expected skipped_targets output, got: %s", content)
	}
	if !strings.Contains(content, "\"branch\":\"release/v0.24\"") {
		t.Fatalf("expected skipped target JSON payload, got: %s", content)
	}
	if !strings.Contains(content, "run_summary<<EOF") {
		t.Fatalf("expected run_summary output, got: %s", content)
	}
}
