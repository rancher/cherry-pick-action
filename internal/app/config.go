package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultLabelPrefix      = "cherry-pick/"
	defaultLogLevel         = "info"
	defaultLogFormat        = "text"
	defaultConflictStrategy = "fail"
	defaultGitUserName      = "Rancher Cherry-Pick Bot"
	defaultGitUserEmail     = "no-reply@rancher.com"
)

var supportedConflictStrategies = map[string]struct{}{
	"fail":           {},
	"placeholder-pr": {},
}

// Config captures runtime options sourced from GitHub Action inputs or environment variables.
type Config struct {
	GitHubToken          string
	GitHubBaseURL        string
	GitHubUploadURL      string
	LabelPrefix          string
	DryRun               bool
	Verbose              bool
	LogLevel             string
	LogFormat            string
	ConflictStrategy     string
	TargetBranches       []string
	GitUserName          string
	GitUserEmail         string
	GitSigningKey        string
	GitSigningPass       string
	RequireOrgMembership bool
}

// LoadConfig reads action inputs from the environment, applies defaults, and performs validation.
func LoadConfig() (Config, error) {
	cfg := Config{
		LabelPrefix:      strings.TrimSpace(envOrDefault("INPUT_LABEL_PREFIX", defaultLabelPrefix)),
		LogLevel:         strings.ToLower(strings.TrimSpace(envOrDefault("INPUT_LOG_LEVEL", defaultLogLevel))),
		LogFormat:        strings.ToLower(strings.TrimSpace(envOrDefault("INPUT_LOG_FORMAT", defaultLogFormat))),
		ConflictStrategy: strings.ToLower(strings.TrimSpace(envOrDefault("INPUT_CONFLICT_STRATEGY", defaultConflictStrategy))),
	}

	cfg.GitHubToken = strings.TrimSpace(os.Getenv("INPUT_GITHUB_TOKEN"))
	if cfg.GitHubToken == "" {
		cfg.GitHubToken = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}

	cfg.GitHubBaseURL = strings.TrimSpace(os.Getenv("INPUT_GITHUB_BASE_URL"))
	cfg.GitHubUploadURL = strings.TrimSpace(os.Getenv("INPUT_GITHUB_UPLOAD_URL"))
	cfg.GitUserName = strings.TrimSpace(os.Getenv("INPUT_GIT_USER_NAME"))
	cfg.GitUserEmail = strings.TrimSpace(os.Getenv("INPUT_GIT_USER_EMAIL"))
	cfg.GitSigningKey = strings.TrimSpace(os.Getenv("INPUT_GIT_SIGNING_KEY"))
	cfg.GitSigningPass = strings.TrimSpace(os.Getenv("INPUT_GIT_SIGNING_PASSPHRASE"))

	if rawTargets := strings.TrimSpace(os.Getenv("INPUT_TARGET_BRANCHES")); rawTargets != "" {
		cfg.TargetBranches = parseBranchList(rawTargets)
	}

	if rawDryRun := strings.TrimSpace(os.Getenv("INPUT_DRY_RUN")); rawDryRun != "" {
		dryRun, err := strconv.ParseBool(rawDryRun)
		if err != nil {
			return Config{}, fmt.Errorf("parse INPUT_DRY_RUN: %w", err)
		}
		cfg.DryRun = dryRun
	}

	if rawVerbose := strings.TrimSpace(os.Getenv("INPUT_VERBOSE")); rawVerbose != "" {
		verbose, err := strconv.ParseBool(rawVerbose)
		if err != nil {
			return Config{}, fmt.Errorf("parse INPUT_VERBOSE: %w", err)
		}
		cfg.Verbose = verbose
	}

	if rawRequireOrg := strings.TrimSpace(os.Getenv("INPUT_REQUIRE_ORG_MEMBERSHIP")); rawRequireOrg != "" {
		requireOrg, err := strconv.ParseBool(rawRequireOrg)
		if err != nil {
			return Config{}, fmt.Errorf("parse INPUT_REQUIRE_ORG_MEMBERSHIP: %w", err)
		}
		cfg.RequireOrgMembership = requireOrg
	}

	if cfg.GitHubToken == "" {
		return Config{}, fmt.Errorf("github token is required (set INPUT_GITHUB_TOKEN or GITHUB_TOKEN)")
	}

	if (cfg.GitHubBaseURL == "") != (cfg.GitHubUploadURL == "") {
		return Config{}, fmt.Errorf("INPUT_GITHUB_BASE_URL and INPUT_GITHUB_UPLOAD_URL must both be set for GitHub Enterprise")
	}

	if cfg.LabelPrefix == "" {
		cfg.LabelPrefix = defaultLabelPrefix
	}

	if cfg.GitUserName == "" {
		cfg.GitUserName = defaultGitUserName
	}

	if cfg.GitUserEmail == "" {
		cfg.GitUserEmail = defaultGitUserEmail
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}

	if cfg.LogFormat == "" {
		cfg.LogFormat = defaultLogFormat
	}

	if cfg.ConflictStrategy == "" {
		cfg.ConflictStrategy = defaultConflictStrategy
	}

	if _, ok := supportedConflictStrategies[cfg.ConflictStrategy]; !ok {
		return Config{}, fmt.Errorf("unsupported conflict strategy %q", cfg.ConflictStrategy)
	}

	supportedFormats := map[string]struct{}{"text": {}, "json": {}}
	if _, ok := supportedFormats[cfg.LogFormat]; !ok {
		return Config{}, fmt.Errorf("unsupported log format %q", cfg.LogFormat)
	}

	if cfg.DryRun && cfg.ConflictStrategy == "placeholder-pr" {
		return Config{}, fmt.Errorf("conflict strategy %q cannot be used when dry run is enabled", cfg.ConflictStrategy)
	}

	if cfg.Verbose {
		cfg.LogLevel = "debug"
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseBranchList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})

	branches := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			branches = append(branches, trimmed)
		}
	}

	return branches
}
