package event

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-github/v55/github"
)

// PullRequestAction enumerates actions we care about from pull_request events.
type PullRequestAction string

const (
	PullRequestActionClosed  PullRequestAction = "closed"
	PullRequestActionLabeled PullRequestAction = "labeled"
)

// PullRequestPayload captures the subset of GitHub pull_request event data used by the action.
type PullRequestPayload struct {
	Action      PullRequestAction
	Repository  Repository
	PullRequest PullRequest
	LabelName   string
}

// Repository identifies the owner/name of the repository where the event originated.
type Repository struct {
	Owner string
	Name  string
}

// PullRequest includes the metadata required for cherry-pick orchestration.
type PullRequest struct {
	Number         int
	Labels         []string
	Merged         bool
	MergeCommitSHA string
	HeadSHA        string
	Title          string
	Body           string
	Assignees      []string
}

// ParsePullRequestEvent decodes a GitHub pull_request event payload from the provided reader.
func ParsePullRequestEvent(r io.Reader) (PullRequestPayload, error) {
	var raw github.PullRequestEvent

	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return PullRequestPayload{}, fmt.Errorf("decode pull_request event: %w", err)
	}

	payload := PullRequestPayload{
		Action: PullRequestAction(strings.ToLower(strings.TrimSpace(raw.GetAction()))),
		Repository: Repository{
			Owner: strings.TrimSpace(raw.GetRepo().GetOwner().GetLogin()),
			Name:  strings.TrimSpace(raw.GetRepo().GetName()),
		},
		PullRequest: PullRequest{
			Number:         raw.GetPullRequest().GetNumber(),
			Merged:         raw.GetPullRequest().GetMerged(),
			MergeCommitSHA: strings.TrimSpace(raw.GetPullRequest().GetMergeCommitSHA()),
			HeadSHA:        strings.TrimSpace(raw.GetPullRequest().GetHead().GetSHA()),
			Title:          raw.GetPullRequest().GetTitle(),
			Body:           raw.GetPullRequest().GetBody(),
		},
	}

	for _, l := range raw.GetPullRequest().Labels {
		if name := strings.TrimSpace(l.GetName()); name != "" {
			payload.PullRequest.Labels = append(payload.PullRequest.Labels, name)
		}
	}

	for _, a := range raw.GetPullRequest().Assignees {
		if login := strings.TrimSpace(a.GetLogin()); login != "" {
			payload.PullRequest.Assignees = append(payload.PullRequest.Assignees, login)
		}
	}

	if raw.Label != nil {
		payload.LabelName = strings.TrimSpace(raw.Label.GetName())
	}

	return payload, nil
}

// ParsePullRequestEventFile reads the event JSON from disk.
func ParsePullRequestEventFile(path string) (PullRequestPayload, error) {
	f, err := os.Open(path)
	if err != nil {
		return PullRequestPayload{}, fmt.Errorf("open event file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			// Log but don't override the return error
			fmt.Fprintf(os.Stderr, "failed to close event file: %v\n", closeErr)
		}
	}()

	payload, err := ParsePullRequestEvent(f)
	if err != nil {
		return PullRequestPayload{}, err
	}

	return payload, nil
}
