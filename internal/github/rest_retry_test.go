package gh

import (
	"errors"
	"net/http"
	"testing"

	github "github.com/google/go-github/v55/github"
)

type stubNetError struct {
	msg       string
	temporary bool
	timeout   bool
}

func (e stubNetError) Error() string   { return e.msg }
func (e stubNetError) Timeout() bool   { return e.timeout }
func (e stubNetError) Temporary() bool { return e.temporary }

func TestClassifyGitHubErrorMarksRateLimitAsRetryable(t *testing.T) {
	original := &github.RateLimitError{Message: "rate limit exceeded"}

	err := classifyGitHubError(original)
	if !IsRetryable(err) {
		t.Fatalf("expected error to be marked retryable")
	}
	if !errors.Is(err, original) {
		t.Fatalf("expected original error to be wrapped")
	}
}

func TestClassifyGitHubErrorMarksHTTP5xxAsRetryable(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusBadGateway}
	original := &github.ErrorResponse{Response: resp}

	err := classifyGitHubError(original)
	if !IsRetryable(err) {
		t.Fatalf("expected 5xx error to be retryable")
	}
	if !errors.Is(err, original) {
		t.Fatalf("expected original error to be wrapped")
	}
}

func TestClassifyGitHubErrorMarksNetworkTimeoutAsRetryable(t *testing.T) {
	original := stubNetError{msg: "timeout", timeout: true}

	err := classifyGitHubError(original)
	if !IsRetryable(err) {
		t.Fatalf("expected timeout error to be retryable")
	}
	if !errors.Is(err, original) {
		t.Fatalf("expected original error to be wrapped")
	}
}

func TestClassifyGitHubErrorLeavesNonRetryableErrorsUntouched(t *testing.T) {
	original := errors.New("fatal error")

	err := classifyGitHubError(original)
	if IsRetryable(err) {
		t.Fatalf("expected error to remain non-retryable")
	}
	if !errors.Is(err, original) {
		t.Fatalf("expected original error to be returned")
	}
}
