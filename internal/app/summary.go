package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rancher/cherry-pick-action/internal/orchestrator"
)

func (r *Runner) writeStepSummary(result orchestrator.Result) error {
	path := strings.TrimSpace(os.Getenv("GITHUB_STEP_SUMMARY"))
	if path == "" {
		return nil
	}

	// Try to ensure directory exists, but don't fail if we can't create it
	// (GitHub Actions should have already set this up)
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			// Log but continue - the file open might still work
			fmt.Fprintf(os.Stderr, "warning: could not create summary directory: %v\n", mkErr)
		}
	}

	var builder strings.Builder
	builder.WriteString("## Cherry-pick action summary\n\n")
	builder.WriteString(renderResultDetails(result))

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open step summary: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log but don't override the return error
			fmt.Fprintf(os.Stderr, "failed to close step summary file: %v\n", closeErr)
		}
	}()

	if _, err := file.WriteString(builder.String()); err != nil {
		return fmt.Errorf("write step summary: %w", err)
	}

	if !strings.HasSuffix(builder.String(), "\n") {
		if _, err := file.WriteString("\n"); err != nil {
			return fmt.Errorf("terminate step summary: %w", err)
		}
	}

	return nil
}

func (r *Runner) writeGitHubOutputs(result orchestrator.Result) error {
	path := strings.TrimSpace(os.Getenv("GITHUB_OUTPUT"))
	if path == "" {
		return nil
	}

	// Try to ensure directory exists, but don't fail if we can't create it
	// (GitHub Actions should have already set this up)
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			// Log but continue - the file open might still work
			fmt.Fprintf(os.Stderr, "warning: could not create outputs directory: %v\n", mkErr)
		}
	}

	created := make([]outputCreatedPR, 0)
	skipped := make([]outputSkippedTarget, 0)

	for _, target := range result.Targets {
		switch target.Status {
		case orchestrator.TargetStatusSucceeded, orchestrator.TargetStatusPlaceholderPR:
			if target.CreatedPR != nil {
				created = append(created, outputCreatedPR{
					Branch: target.Target.Branch,
					Number: target.CreatedPR.Number,
					URL:    target.CreatedPR.URL,
					Head:   target.CreatedPR.Head,
					Base:   target.CreatedPR.Base,
				})
			}
		case orchestrator.TargetStatusSkippedNoBranch,
			orchestrator.TargetStatusSkippedExistingPR,
			orchestrator.TargetStatusSkippedAlreadyPresent,
			orchestrator.TargetStatusFailed,
			orchestrator.TargetStatusDryRun:
			skipped = append(skipped, outputSkippedTarget{
				Branch: target.Target.Branch,
				Status: string(target.Status),
				Reason: target.Reason,
			})
		}
	}

	createdJSON, err := json.Marshal(created)
	if err != nil {
		return fmt.Errorf("marshal created_prs: %w", err)
	}

	skippedJSON, err := json.Marshal(skipped)
	if err != nil {
		return fmt.Errorf("marshal skipped_targets: %w", err)
	}

	summary := struct {
		Skipped       bool   `json:"skipped"`
		SkippedReason string `json:"skipped_reason"`
	}{Skipped: result.Skipped, SkippedReason: result.SkippedReason}

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal run_summary: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open github output: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log but don't override the return error
			fmt.Fprintf(os.Stderr, "failed to close github output file: %v\n", closeErr)
		}
	}()

	if err := writeMultilineOutput(file, "created_prs", string(createdJSON)); err != nil {
		return err
	}

	if err := writeMultilineOutput(file, "skipped_targets", string(skippedJSON)); err != nil {
		return err
	}

	if err := writeMultilineOutput(file, "run_summary", string(summaryJSON)); err != nil {
		return err
	}

	return nil
}

func renderResultDetails(result orchestrator.Result) string {
	var builder strings.Builder

	if result.Skipped {
		reason := result.SkippedReason
		if reason == "" {
			reason = "run skipped"
		}
		builder.WriteString(fmt.Sprintf("Skipped cherry-pick orchestration: %s\n", sanitizeMarkdownCell(reason)))
		return builder.String()
	}

	if len(result.Targets) == 0 {
		builder.WriteString("No cherry-pick targets were evaluated.\n")
		return builder.String()
	}

	builder.WriteString("| Branch | Status | Details | PR |\n")
	builder.WriteString("| --- | --- | --- | --- |\n")
	for _, target := range result.Targets {
		status := string(target.Status)
		details := target.Reason
		if details == "" {
			details = "-"
		}

		prCell := "-"
		if target.CreatedPR != nil {
			if target.CreatedPR.URL != "" {
				prCell = fmt.Sprintf("[PR #%d](%s)", target.CreatedPR.Number, target.CreatedPR.URL)
			} else {
				prCell = fmt.Sprintf("PR #%d", target.CreatedPR.Number)
			}
		} else if target.ExistingPR != nil && target.ExistingPR.URL != "" {
			prCell = fmt.Sprintf("[Existing #%d](%s)", target.ExistingPR.Number, target.ExistingPR.URL)
		}

		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			sanitizeMarkdownCell(target.Target.Branch),
			sanitizeMarkdownCell(status),
			sanitizeMarkdownCell(details),
			sanitizeMarkdownCell(prCell),
		))
	}

	return builder.String()
}

type outputCreatedPR struct {
	Branch string `json:"branch"`
	Number int    `json:"number"`
	URL    string `json:"url"`
	Head   string `json:"head"`
	Base   string `json:"base"`
}

type outputSkippedTarget struct {
	Branch string `json:"branch"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func writeMultilineOutput(file *os.File, key, value string) error {
	if _, err := fmt.Fprintf(file, "%s<<EOF\n%s\nEOF\n", key, value); err != nil {
		return fmt.Errorf("write output %s: %w", key, err)
	}
	return nil
}

func sanitizeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
