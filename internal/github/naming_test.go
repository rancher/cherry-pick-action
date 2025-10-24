package gh

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestBranchNameForCherryPick(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		pr         int
		opts       []BranchNamingOptions
		expect     string
		maxLen     int
		expectHash bool
	}{
		{
			name:   "simple",
			target: "release/v0.25",
			pr:     123,
			expect: "cherry-pick/release/v0.25/pr-123",
		},
		{
			name:   "multiple slashes in branch name",
			target: "release/v2.9/security",
			pr:     456,
			expect: "cherry-pick/release/v2.9/security/pr-456",
		},
		{
			name:   "deeply nested branch with slashes",
			target: "feature/team/project/branch",
			pr:     789,
			expect: "cherry-pick/feature/team/project/branch/pr-789",
		},
		{
			name:   "branch with trailing slash",
			target: "release/v0.25/",
			pr:     100,
			expect: "cherry-pick/release/v0.25/pr-100",
		},
		{
			name:   "branch with leading slash",
			target: "/release/v0.25",
			pr:     200,
			expect: "cherry-pick/release/v0.25/pr-200",
		},
		{
			name:   "uppercase and spaces",
			target: "Release / V0.26",
			pr:     50,
			expect: "cherry-pick/release/v0.26/pr-50",
		},
		{
			name:   "disallowed characters replaced",
			target: "release@v0#27",
			pr:     75,
			expect: "cherry-pick/release-v0-27/pr-75",
		},
		{
			name:   "empty target replaced",
			target: " ",
			pr:     10,
			expect: "cherry-pick/target/pr-10",
		},
		{
			name:       "extremely long target truncated with hash",
			target:     strings.Repeat("release-", 10) + "v1",
			pr:         999,
			maxLen:     63,
			expectHash: true,
		},
		{
			name:   "custom options",
			target: "Release-Branch",
			pr:     5,
			opts: []BranchNamingOptions{{
				Prefix:     "custom",
				MaxLength:  40,
				HashLength: 6,
			}},
			maxLen:     40,
			expectHash: false,
		},
	}

	hashSuffixPattern := regexp.MustCompile(`-[0-9a-f]{6,8}$`)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			branch := BranchNameForCherryPick(tc.target, tc.pr, tc.opts...)

			if tc.expect != "" {
				if branch != tc.expect {
					t.Fatalf("expected %q, got %q", tc.expect, branch)
				}
				return
			}

			limit := tc.maxLen
			if limit == 0 {
				limit = defaultBranchNaming.MaxLength
			}

			if len(branch) > limit {
				t.Fatalf("expected branch length <= %d, got %d (%q)", limit, len(branch), branch)
			}

			if !strings.Contains(branch, fmt.Sprintf("/pr-%d", tc.pr)) {
				t.Fatalf("expected branch to contain PR segment, got %q", branch)
			}

			parts := strings.Split(branch, "/")
			if len(parts) != 3 {
				t.Fatalf("expected branch to have prefix/target/pr segments, got %q", branch)
			}

			targetSegment := parts[1]

			if tc.expectHash {
				if !hashSuffixPattern.MatchString(targetSegment) {
					t.Fatalf("expected truncated target to end with hash suffix, got %q", targetSegment)
				}
			}

			if len(tc.opts) > 0 {
				if !strings.HasPrefix(branch, "custom/") {
					t.Fatalf("expected custom prefix, got %q", branch)
				}
			}

			if branch != strings.ToLower(branch) {
				t.Fatalf("expected branch to be lowercase, got %q", branch)
			}
		})
	}
}
