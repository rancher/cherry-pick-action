package gh

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
)

var disallowedBranchChars = regexp.MustCompile(`[^a-zA-Z0-9._/-]+`)

// BranchNamingOptions controls how cherry-pick branch names are generated.
type BranchNamingOptions struct {
	Prefix            string
	MaxLength         int
	HashLength        int
	SanitizeEmptyWith string
}

var defaultBranchNaming = BranchNamingOptions{
	Prefix:            "cherry-pick",
	MaxLength:         63,
	HashLength:        8,
	SanitizeEmptyWith: "target",
}

// BranchNameForCherryPick computes a branch name for the cherry-pick operation,
// ensuring the target branch portion is sanitized and length-limited. Optional
// BranchNamingOptions can be supplied to tweak the naming behavior.
func BranchNameForCherryPick(targetBranch string, sourcePR int, opts ...BranchNamingOptions) string {
	config := defaultBranchNaming
	if len(opts) > 0 {
		o := opts[0]
		if o.Prefix != "" {
			config.Prefix = o.Prefix
		}
		if o.MaxLength > 0 {
			config.MaxLength = o.MaxLength
		}
		if o.HashLength > 0 {
			config.HashLength = o.HashLength
		}
		if o.SanitizeEmptyWith != "" {
			config.SanitizeEmptyWith = o.SanitizeEmptyWith
		}
	}

	sanitized := sanitizeBranchSegment(targetBranch, config)
	prSegment := fmt.Sprintf("pr-%d", sourcePR)
	branch := fmt.Sprintf("%s/%s/%s", config.Prefix, sanitized, prSegment)

	if len(branch) <= config.MaxLength {
		return branch
	}

	available := config.MaxLength - len(config.Prefix) - 1 - len(prSegment) - 1
	if available < 1 {
		available = 1
	}

	shortened := shortenTargetSegment(sanitized, available, config)
	return fmt.Sprintf("%s/%s/%s", config.Prefix, shortened, prSegment)
}

func sanitizeBranchSegment(segment string, config BranchNamingOptions) string {
	segment = strings.TrimSpace(segment)
	segment = strings.ReplaceAll(segment, " ", "-")
	segment = disallowedBranchChars.ReplaceAllString(segment, "-")
	segment = strings.Trim(segment, "-/.")

	if segment == "" {
		segment = config.SanitizeEmptyWith
	}

	segment = strings.ToLower(segment)
	segment = strings.ReplaceAll(segment, "-/-", "/")
	for strings.Contains(segment, "//") {
		segment = strings.ReplaceAll(segment, "//", "/")
	}
	for strings.Contains(segment, "--") {
		segment = strings.ReplaceAll(segment, "--", "-")
	}
	segment = strings.Trim(segment, "-")

	if segment == "" {
		segment = config.SanitizeEmptyWith
	}

	return segment
}

func shortenTargetSegment(segment string, available int, config BranchNamingOptions) string {
	if available <= 0 {
		return config.SanitizeEmptyWith
	}

	if len(segment) <= available {
		return segment
	}

	hashLen := config.HashLength
	if hashLen <= 0 {
		hashLen = 8
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(segment))
	hex := fmt.Sprintf("%0*x", hashLen, h.Sum32())
	suffix := "-" + hex

	if len(suffix) > available {
		// Not enough room for hyphen + full hash; fall back to hash prefix.
		if len(hex) >= available {
			return hex[:available]
		}
		return hex
	}

	baseLen := available - len(suffix)
	if baseLen <= 0 {
		return suffix[len(suffix)-available:]
	}

	base := segment
	if len(base) > baseLen {
		base = segment[:baseLen]
	}

	base = strings.TrimRight(base, "-./")
	if base == "" {
		fallback := config.SanitizeEmptyWith
		if len(fallback) > baseLen {
			fallback = fallback[:baseLen]
		}
		base = fallback
	}

	return strings.TrimRight(base, "-./") + suffix
}
