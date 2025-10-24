package event_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/cherry-pick-action/internal/event"
)

var _ = Describe("ParsePullRequestEvent", func() {
	const sample = `{
		"action": "labeled",
		"label": {"name": "cherry-pick/release/v0.25"},
		"repository": {
			"name": "cherry-pick-action",
			"owner": {"login": "rancher"}
		},
		"pull_request": {
			"number": 123,
			"merged": true,
			"merge_commit_sha": "abc123",
			"title": "Fix bug",
			"body": "Body text",
			"head": {"sha": "def456"},
			"labels": [
				{"name": "cherry-pick/release/v0.25"},
				{"name": "kind/bug"}
			],
			"assignees": [
				{"login": "alice"},
				{"login": "bob"}
			]
		}
	}`

	It("parses repository and pull request details", func() {
		payload, err := event.ParsePullRequestEvent(strings.NewReader(sample))
		Expect(err).NotTo(HaveOccurred())

		Expect(payload.Action).To(Equal(event.PullRequestActionLabeled))
		Expect(payload.Repository.Owner).To(Equal("rancher"))
		Expect(payload.Repository.Name).To(Equal("cherry-pick-action"))

		pr := payload.PullRequest
		Expect(pr.Number).To(Equal(123))
		Expect(pr.Merged).To(BeTrue())
		Expect(pr.MergeCommitSHA).To(Equal("abc123"))
		Expect(pr.HeadSHA).To(Equal("def456"))
		Expect(pr.Title).To(Equal("Fix bug"))
		Expect(pr.Body).To(Equal("Body text"))
		Expect(pr.Labels).To(ConsistOf("cherry-pick/release/v0.25", "kind/bug"))
		Expect(pr.Assignees).To(ConsistOf("alice", "bob"))
		Expect(payload.LabelName).To(Equal("cherry-pick/release/v0.25"))
	})

	It("normalizes empty fields", func() {
		emptyPayload, err := event.ParsePullRequestEvent(strings.NewReader(`{"action":"CLOSED","repository":{"name":"repo","owner":{"login":"ORG"}},"pull_request":{"number":1,"merged":false,"head":{"sha":""}}}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(emptyPayload.Action).To(Equal(event.PullRequestAction("closed")))
		Expect(emptyPayload.Repository.Owner).To(Equal("ORG"))
		Expect(emptyPayload.PullRequest.Labels).To(BeEmpty())
	})
})
