package gh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	github "github.com/google/go-github/v55/github"
	"golang.org/x/oauth2"
)

const defaultUserAgent = "rancher-cherry-pick-action"

// NewRESTFactory returns a GitHub client factory backed by the go-github REST client. When
// base and upload URLs are provided, the factory targets a GitHub Enterprise instance.
func NewRESTFactory(baseURL, uploadURL string) Factory {
	return &restFactory{
		userAgent: defaultUserAgent,
		baseURL:   strings.TrimSpace(baseURL),
		uploadURL: strings.TrimSpace(uploadURL),
	}
}

type restFactory struct {
	userAgent string
	baseURL   string
	uploadURL string
}

type restClient struct {
	client *github.Client
}

func (f *restFactory) New(ctx context.Context, token string) (Client, error) {
	if token == "" {
		return nil, fmt.Errorf("github token is required")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	if f.baseURL == "" && f.uploadURL != "" {
		return nil, fmt.Errorf("github upload url cannot be set without base url")
	}

	var ghClient *github.Client
	if f.baseURL != "" {
		baseURLNormalized, err := normalizeGitHubURL(f.baseURL)
		if err != nil {
			return nil, fmt.Errorf("parse github base url: %w", err)
		}

		uploadURL := f.uploadURL
		if uploadURL == "" {
			return nil, fmt.Errorf("github upload url must be provided when base url is set")
		}

		uploadURLNormalized, err := normalizeGitHubURL(uploadURL)
		if err != nil {
			return nil, fmt.Errorf("parse github upload url: %w", err)
		}

		ghClient, err = github.NewClient(tc).WithEnterpriseURLs(baseURLNormalized, uploadURLNormalized)
		if err != nil {
			return nil, fmt.Errorf("construct enterprise github client: %w", err)
		}
	} else {
		ghClient = github.NewClient(tc)
	}

	if f.userAgent != "" {
		ghClient.UserAgent = f.userAgent
	}

	return &restClient{client: ghClient}, nil
}

func normalizeGitHubURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url cannot be empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	if parsed.Scheme == "" {
		return "", fmt.Errorf("url must include scheme (e.g. https://)")
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("url must include host")
	}

	if parsed.Path == "" {
		parsed.Path = "/"
	} else if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String(), nil
}

func (c *restClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (PRMetadata, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		err = classifyGitHubError(err)
		return PRMetadata{}, fmt.Errorf("get pull request: %w", err)
	}

	labels := make([]string, 0, len(pr.Labels))
	for _, label := range pr.Labels {
		if label == nil {
			continue
		}
		if name := label.GetName(); name != "" {
			labels = append(labels, name)
		}
	}

	assignees := make([]string, 0, len(pr.Assignees))
	for _, user := range pr.Assignees {
		if user == nil {
			continue
		}
		if login := user.GetLogin(); login != "" {
			assignees = append(assignees, login)
		}
	}

	metadata := PRMetadata{
		Owner:     owner,
		Repo:      repo,
		Number:    pr.GetNumber(),
		Title:     pr.GetTitle(),
		Body:      pr.GetBody(),
		MergeSHA:  pr.GetMergeCommitSHA(),
		Labels:    labels,
		Assignees: assignees,
		IsMerged:  pr.GetMerged(),
	}

	if head := pr.GetHead(); head != nil {
		metadata.HeadSHA = head.GetSHA()
		metadata.HeadRef = head.GetRef()
		if headRepo := head.GetRepo(); headRepo != nil {
			metadata.HeadRepo = headRepo.GetName()
			if owner := headRepo.GetOwner(); owner != nil {
				metadata.HeadOwner = owner.GetLogin()
			}
		}
	}

	if metadata.HeadOwner != "" && !strings.EqualFold(metadata.HeadOwner, owner) {
		metadata.IsFromFork = true
	}

	if metadata.HeadRepo != "" && !strings.EqualFold(metadata.HeadRepo, repo) {
		metadata.IsFromFork = true
	}

	return metadata, nil
}

func (c *restClient) ListCherryPickPRs(ctx context.Context, owner, repo string, sourcePR int, targetBranch string) ([]CherryPickPR, error) {
	branchName := BranchNameForCherryPick(targetBranch, sourcePR)
	opts := &github.PullRequestListOptions{
		State: "all",
		Head:  fmt.Sprintf("%s:%s", owner, branchName),
		Base:  targetBranch,
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
	}

	var results []CherryPickPR
	for {
		prs, resp, err := c.client.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			err = classifyGitHubError(err)
			return nil, fmt.Errorf("list pull requests: %w", err)
		}

		for _, pr := range prs {
			if pr == nil {
				continue
			}
			result := CherryPickPR{
				URL:    pr.GetHTMLURL(),
				Number: pr.GetNumber(),
			}
			if head := pr.GetHead(); head != nil {
				result.Head = head.GetRef()
			}
			if base := pr.GetBase(); base != nil {
				result.Base = base.GetRef()
			}
			results = append(results, result)
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return results, nil
}

func (c *restClient) EnsureBranchExists(ctx context.Context, owner, repo, branch string) error {
	_, resp, err := c.client.Repositories.GetBranch(ctx, owner, repo, branch, false)
	if err != nil {
		if isNotFound(resp, err) {
			return ErrBranchNotFound
		}
		err = classifyGitHubError(err)
		return fmt.Errorf("get branch %s: %w", branch, err)
	}
	return nil
}

func (c *restClient) CreateBranch(ctx context.Context, owner, repo, branch, fromSHA string) error {
	ref := &github.Reference{
		Ref:    github.String(fmt.Sprintf("refs/heads/%s", branch)),
		Object: &github.GitObject{SHA: github.String(fromSHA)},
	}

	if _, _, err := c.client.Git.CreateRef(ctx, owner, repo, ref); err != nil {
		err = classifyGitHubError(err)
		return fmt.Errorf("create ref %s: %w", branch, err)
	}
	return nil
}

func (c *restClient) CreatePullRequest(ctx context.Context, owner, repo string, input CreatePROptions) (CherryPickPR, error) {
	pr, _, err := c.client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title:               github.String(input.Title),
		Head:                github.String(input.Head),
		Base:                github.String(input.Base),
		Body:                github.String(input.Body),
		Draft:               github.Bool(input.Draft),
		MaintainerCanModify: github.Bool(input.MaintainerCanModify),
	})
	if err != nil {
		err = classifyGitHubError(err)
		return CherryPickPR{}, fmt.Errorf("create pull request: %w", err)
	}

	result := CherryPickPR{
		URL:    pr.GetHTMLURL(),
		Number: pr.GetNumber(),
	}
	if head := pr.GetHead(); head != nil {
		result.Head = head.GetRef()
	}
	if base := pr.GetBase(); base != nil {
		result.Base = base.GetRef()
	}

	if len(input.Labels) > 0 {
		_, _, err = c.client.Issues.AddLabelsToIssue(ctx, owner, repo, pr.GetNumber(), input.Labels)
		if err != nil {
			err = classifyGitHubError(err)
			return result, fmt.Errorf("add labels to pull request: %w", err)
		}
	}

	if len(input.Assignees) > 0 {
		_, _, err = c.client.Issues.AddAssignees(ctx, owner, repo, pr.GetNumber(), input.Assignees)
		if err != nil {
			err = classifyGitHubError(err)
			return result, fmt.Errorf("add assignees to pull request: %w", err)
		}
	}

	return result, nil
}

func (c *restClient) CommentOnPullRequest(ctx context.Context, owner, repo string, number int, body string) error {
	comment := &github.IssueComment{Body: github.String(body)}
	if _, _, err := c.client.Issues.CreateComment(ctx, owner, repo, number, comment); err != nil {
		err = classifyGitHubError(err)
		return fmt.Errorf("create comment: %w", err)
	}
	return nil
}

func (c *restClient) CommitExistsOnBranch(ctx context.Context, owner, repo, commitSHA, branch string) (bool, error) {
	comp, resp, err := c.client.Repositories.CompareCommits(ctx, owner, repo, branch, commitSHA, nil)
	if err != nil {
		if isNotFound(resp, err) {
			return false, nil
		}
		err = classifyGitHubError(err)
		return false, fmt.Errorf("compare commits %s..%s: %w", branch, commitSHA, err)
	}

	status := comp.GetStatus()
	switch status {
	case "behind", "identical":
		return true, nil
	default:
		return false, nil
	}
}

func (c *restClient) ListPullRequestComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	opts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	var results []IssueComment

	for {
		comments, resp, err := c.client.Issues.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			err = classifyGitHubError(err)
			return nil, fmt.Errorf("list comments: %w", err)
		}

		for _, comment := range comments {
			if comment == nil {
				continue
			}
			results = append(results, IssueComment{ID: comment.GetID(), Body: comment.GetBody()})
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return results, nil
}

func (c *restClient) UpdateComment(ctx context.Context, owner, repo string, commentID int64, body string) error {
	comment := &github.IssueComment{Body: github.String(body)}
	if _, _, err := c.client.Issues.EditComment(ctx, owner, repo, commentID, comment); err != nil {
		err = classifyGitHubError(err)
		return fmt.Errorf("edit comment: %w", err)
	}
	return nil
}

func isNotFound(resp *github.Response, err error) bool {
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return true
	}
	var githubErr *github.ErrorResponse
	if errors.As(err, &githubErr) {
		if githubErr.Response != nil && githubErr.Response.StatusCode == http.StatusNotFound {
			return true
		}
	}
	return false
}

func (c *restClient) HasLabel(ctx context.Context, owner, repo string, number int, label string) (bool, error) {
	// List all labels on the PR and check if the target label exists
	labels, _, err := c.client.Issues.ListLabelsByIssue(ctx, owner, repo, number, &github.ListOptions{PerPage: 100})
	if err != nil {
		return false, classifyGitHubError(err)
	}

	for _, l := range labels {
		if l.GetName() == label {
			return true, nil
		}
	}

	return false, nil
}

func (c *restClient) AddLabel(ctx context.Context, owner, repo string, number int, label string) error {
	_, _, err := c.client.Issues.AddLabelsToIssue(ctx, owner, repo, number, []string{label})
	return classifyGitHubError(err)
}

func (c *restClient) CheckOrgMembership(ctx context.Context, org, username string) (bool, error) {
	_, resp, err := c.client.Organizations.GetOrgMembership(ctx, username, org)
	if err != nil {
		if isNotFound(resp, err) {
			return false, nil
		}
		return false, classifyGitHubError(err)
	}
	return true, nil
}

func classifyGitHubError(err error) error {
	if err == nil {
		return nil
	}
	if isRetryableGitHubError(err) {
		return &retryableError{err: err}
	}
	return err
}

func isRetryableGitHubError(err error) bool {
	if err == nil {
		return false
	}

	var rateLimitErr *github.RateLimitError
	if errors.As(err, &rateLimitErr) {
		return true
	}

	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		return true
	}

	var acceptedErr *github.AcceptedError
	if errors.As(err, &acceptedErr) {
		return true
	}

	var respErr *github.ErrorResponse
	if errors.As(err, &respErr) {
		if respErr.Response != nil {
			code := respErr.Response.StatusCode
			if code == http.StatusTooManyRequests || (code >= 500 && code <= 599) {
				return true
			}
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	return false
}
