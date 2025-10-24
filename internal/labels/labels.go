package labels

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Target represents a release branch derived from a cherry-pick label.
type Target struct {
	LabelName string
	Branch    string
}

var (
	errEmptyPrefix = errors.New("label prefix cannot be empty")
)

// CollectTargets scans the provided label names, extracts those that match the given
// prefix, and returns deduplicated Target entries (preserving first-seen order).
func CollectTargets(labelNames []string, prefix string) ([]Target, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil, errEmptyPrefix
	}

	targets := make([]Target, 0, len(labelNames))
	seen := make(map[string]struct{})

	for _, name := range labelNames {
		branch, ok := parseBranch(name, prefix)
		if !ok {
			continue
		}

		if _, exists := seen[branch]; exists {
			continue
		}

		seen[branch] = struct{}{}
		targets = append(targets, Target{LabelName: name, Branch: branch})
	}

	return targets, nil
}

// parseBranch returns the normalized branch if the label matches the prefix.
func parseBranch(labelName, prefix string) (string, bool) {
	labelName = strings.TrimSpace(labelName)
	if labelName == "" {
		return "", false
	}

	if !strings.HasPrefix(strings.ToLower(labelName), strings.ToLower(prefix)) {
		return "", false
	}

	branch := NormalizeBranch(labelName[len(prefix):])

	if branch == "" {
		return "", false
	}

	return branch, true
}

// ValidateTargets ensures each target branch conforms to simple safety checks.
func ValidateTargets(targets []Target) error {
	for _, t := range targets {
		if err := validateBranchName(t.Branch); err != nil {
			return fmt.Errorf("invalid branch %q from label %q: %w", t.Branch, t.LabelName, err)
		}
	}
	return nil
}

func validateBranchName(branch string) error {
	if branch == "" {
		return errors.New("branch cannot be empty")
	}

	if strings.ContainsAny(branch, " \t\n\r") {
		return errors.New("branch cannot contain whitespace")
	}

	if strings.Contains(branch, "..") {
		return errors.New("branch cannot contain '..'")
	}

	if strings.ContainsAny(branch, "~^:?*[]@{\\") {
		return errors.New("branch contains forbidden git characters")
	}

	return nil
}

// MergeTargets merges multiple slices of targets preserving order and removing duplicates.
func MergeTargets(groups ...[]Target) []Target {
	result := make([]Target, 0)
	seen := make(map[string]struct{})

	for _, group := range groups {
		for _, t := range group {
			if _, ok := seen[t.Branch]; ok {
				continue
			}
			seen[t.Branch] = struct{}{}
			result = append(result, t)
		}
	}

	return result
}

// Branches returns the branch names extracted from the targets.
func Branches(targets []Target) []string {
	branches := make([]string, 0, len(targets))
	for _, t := range targets {
		branches = append(branches, t.Branch)
	}
	return branches
}

// SortedBranches returns a deduplicated, sorted list of branch names.
func SortedBranches(targets []Target) []string {
	branches := Branches(targets)
	slices.Sort(branches)
	return slices.Compact(branches)
}

// NormalizeBranch trims whitespace, removes leading/trailing slashes, and strips
// refs/heads prefixes from a branch name. It returns an empty string when the
// normalized branch would otherwise be empty.
func NormalizeBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	branch = strings.Trim(branch, "/")

	if len(branch) >= len("refs/heads/") && strings.EqualFold(branch[:len("refs/heads/")], "refs/heads/") {
		branch = branch[len("refs/heads/"):]
	}

	branch = strings.TrimSpace(branch)
	branch = strings.Trim(branch, "/")

	return strings.TrimSpace(branch)
}
