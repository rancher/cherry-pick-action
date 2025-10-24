package app

import (
	"os"
	"testing"
)

func TestLoadConfigRejectsPlaceholderWithDryRun(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_CONFLICT_STRATEGY", "placeholder-pr")
	t.Setenv("INPUT_DRY_RUN", "true")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_CONFLICT_STRATEGY")
		_ = os.Unsetenv("INPUT_DRY_RUN")
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
	})

	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected error when using placeholder-pr with dry run")
	}
}

func TestLoadConfigEnterpriseURLs(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_GITHUB_BASE_URL", "https://github.example.com/api/v3")
	t.Setenv("INPUT_GITHUB_UPLOAD_URL", "https://github.example.com/uploads")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_GITHUB_BASE_URL")
		_ = os.Unsetenv("INPUT_GITHUB_UPLOAD_URL")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.GitHubBaseURL != "https://github.example.com/api/v3" {
		t.Fatalf("expected base URL to be preserved, got %q", cfg.GitHubBaseURL)
	}

	if cfg.GitHubUploadURL != "https://github.example.com/uploads" {
		t.Fatalf("expected upload URL to be preserved, got %q", cfg.GitHubUploadURL)
	}
}

func TestLoadConfigEnterpriseURLMismatch(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_GITHUB_BASE_URL", "https://github.example.com/api/v3")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_GITHUB_BASE_URL")
	})

	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected error when only base URL is provided")
	}
}

func TestLoadConfigLogFormatDefault(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.LogFormat != "text" {
		t.Fatalf("expected default log format text, got %q", cfg.LogFormat)
	}
}

func TestLoadConfigLogFormatValidation(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_LOG_FORMAT", "invalid")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_LOG_FORMAT")
	})

	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected error for unsupported log format")
	}
}

func TestLoadConfigVerboseForcesDebugLevel(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_VERBOSE", "true")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_VERBOSE")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if !cfg.Verbose {
		t.Fatalf("expected verbose flag to be true")
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf("expected verbose mode to force debug log level, got %q", cfg.LogLevel)
	}
}

func TestLoadConfigParsesTargetBranches(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_TARGET_BRANCHES", "release/v0.25, release/v0.24\nrelease/v0.23")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_TARGET_BRANCHES")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	expected := []string{"release/v0.25", "release/v0.24", "release/v0.23"}
	if len(cfg.TargetBranches) != len(expected) {
		t.Fatalf("expected %d target branches, got %d", len(expected), len(cfg.TargetBranches))
	}
	for i, branch := range expected {
		if cfg.TargetBranches[i] != branch {
			t.Fatalf("expected branch %d to be %q, got %q", i, branch, cfg.TargetBranches[i])
		}
	}
}

func TestLoadConfigSetsDefaultGitIdentity(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.GitUserName != defaultGitUserName {
		t.Fatalf("expected default git user name %q, got %q", defaultGitUserName, cfg.GitUserName)
	}

	if cfg.GitUserEmail != defaultGitUserEmail {
		t.Fatalf("expected default git user email %q, got %q", defaultGitUserEmail, cfg.GitUserEmail)
	}
}

func TestLoadConfigHonorsGitIdentityOverrides(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_GIT_USER_NAME", "Automator")
	t.Setenv("INPUT_GIT_USER_EMAIL", "automator@example.com")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_GIT_USER_NAME")
		_ = os.Unsetenv("INPUT_GIT_USER_EMAIL")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.GitUserName != "Automator" {
		t.Fatalf("expected git user name override, got %q", cfg.GitUserName)
	}

	if cfg.GitUserEmail != "automator@example.com" {
		t.Fatalf("expected git user email override, got %q", cfg.GitUserEmail)
	}
}

func TestLoadConfigRequireOrgMembership(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_REQUIRE_ORG_MEMBERSHIP", "true")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_REQUIRE_ORG_MEMBERSHIP")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if !cfg.RequireOrgMembership {
		t.Fatalf("expected RequireOrgMembership to be true")
	}
}

func TestLoadConfigRequireOrgMembershipDefault(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.RequireOrgMembership {
		t.Fatalf("expected RequireOrgMembership to be false by default")
	}
}
