package app

import (
	"os"
	"testing"
)

func TestLoadConfigWithGPGSigning(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Setenv("INPUT_GIT_SIGNING_KEY", "-----BEGIN PGP PRIVATE KEY BLOCK-----\nfakekey\n-----END PGP PRIVATE KEY BLOCK-----")
	t.Setenv("INPUT_GIT_SIGNING_PASSPHRASE", "secret")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
		_ = os.Unsetenv("INPUT_GIT_SIGNING_KEY")
		_ = os.Unsetenv("INPUT_GIT_SIGNING_PASSPHRASE")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.GitSigningKey == "" {
		t.Fatalf("expected git signing key to be loaded")
	}

	if cfg.GitSigningPass != "secret" {
		t.Fatalf("expected git signing passphrase to be loaded, got %q", cfg.GitSigningPass)
	}
}

func TestLoadConfigWithoutGPGSigning(t *testing.T) {
	t.Setenv("INPUT_GITHUB_TOKEN", "token")
	t.Cleanup(func() {
		_ = os.Unsetenv("INPUT_GITHUB_TOKEN")
	})

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.GitSigningKey != "" {
		t.Fatalf("expected no git signing key when not provided, got %q", cfg.GitSigningKey)
	}

	if cfg.GitSigningPass != "" {
		t.Fatalf("expected no git signing passphrase when not provided, got %q", cfg.GitSigningPass)
	}
}
