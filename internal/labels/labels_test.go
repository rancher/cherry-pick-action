package labels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/cherry-pick-action/internal/labels"
)

var _ = Describe("Labels", func() {
	Describe("CollectTargets", func() {
		It("extracts unique branches matching the prefix", func() {
			labelNames := []string{
				"enhancement",
				"cherry-pick/release/v0.25",
				"Cherry-Pick/release/v0.24",
				"cherry-pick/release/v0.25",
				"cherry-pick/",
				"cherry-pick/release/v0.24 ",
			}

			targets, err := labels.CollectTargets(labelNames, "cherry-pick/")
			Expect(err).NotTo(HaveOccurred())
			Expect(targets).To(HaveLen(2))
			Expect(targets[0].Branch).To(Equal("release/v0.25"))
			Expect(targets[1].Branch).To(Equal("release/v0.24"))
		})

		It("handles labels with multiple slashes in branch names", func() {
			labelNames := []string{
				"cherry-pick/release/v0.25",
				"cherry-pick/feature/foo/bar",
				"cherry-pick/release/v2.9/security",
				"cherry-pick/main",
			}

			targets, err := labels.CollectTargets(labelNames, "cherry-pick/")
			Expect(err).NotTo(HaveOccurred())
			Expect(targets).To(HaveLen(4))
			Expect(targets[0].Branch).To(Equal("release/v0.25"))
			Expect(targets[1].Branch).To(Equal("feature/foo/bar"))
			Expect(targets[2].Branch).To(Equal("release/v2.9/security"))
			Expect(targets[3].Branch).To(Equal("main"))
		})

		It("returns an error when the prefix is empty", func() {
			_, err := labels.CollectTargets([]string{"cherry-pick/release"}, " ")
			Expect(err).To(HaveOccurred())
		})

		It("normalizes branch names with refs prefix and stray slashes", func() {
			labelNames := []string{"cherry-pick/ refs/heads/release/v0.30//"}

			targets, err := labels.CollectTargets(labelNames, "cherry-pick/")
			Expect(err).NotTo(HaveOccurred())
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].Branch).To(Equal("release/v0.30"))
		})
	})

	Describe("ValidateTargets", func() {
		It("accepts valid branch names", func() {
			targets := []labels.Target{{LabelName: "cherry-pick/release/v0.25", Branch: "release/v0.25"}}
			Expect(labels.ValidateTargets(targets)).To(Succeed())
		})

		It("accepts branch names with multiple slashes", func() {
			targets := []labels.Target{
				{LabelName: "cherry-pick/release/v0.25", Branch: "release/v0.25"},
				{LabelName: "cherry-pick/feature/foo/bar", Branch: "feature/foo/bar"},
				{LabelName: "cherry-pick/release/v2.9/security", Branch: "release/v2.9/security"},
			}
			Expect(labels.ValidateTargets(targets)).To(Succeed())
		})

		It("rejects invalid branch names", func() {
			targets := []labels.Target{{LabelName: "bad", Branch: "feature with space"}}
			err := labels.ValidateTargets(targets)
			Expect(err).To(HaveOccurred())
		})

		It("rejects branch names with forbidden characters", func() {
			invalidTargets := []labels.Target{
				{LabelName: "bad1", Branch: "feature..bad"},
				{LabelName: "bad2", Branch: "feature~bad"},
				{LabelName: "bad3", Branch: "feature^bad"},
				{LabelName: "bad4", Branch: "feature:bad"},
			}
			for _, target := range invalidTargets {
				err := labels.ValidateTargets([]labels.Target{target})
				Expect(err).To(HaveOccurred(), "Expected branch %q to be invalid", target.Branch)
			}
		})
	})

	Describe("MergeTargets", func() {
		It("deduplicates branches while preserving first-seen order", func() {
			a := []labels.Target{{LabelName: "a", Branch: "release/v0.26"}}
			b := []labels.Target{{LabelName: "b", Branch: "release/v0.25"}, {LabelName: "c", Branch: "release/v0.26"}}

			merged := labels.MergeTargets(a, b)
			Expect(merged).To(HaveLen(2))
			Expect(merged[0].Branch).To(Equal("release/v0.26"))
			Expect(merged[1].Branch).To(Equal("release/v0.25"))
		})
	})

	Describe("SortedBranches", func() {
		It("returns a sorted, deduplicated slice", func() {
			targets := []labels.Target{
				{Branch: "release/v0.26"},
				{Branch: "release/v0.24"},
				{Branch: "release/v0.25"},
				{Branch: "release/v0.24"},
			}

			branches := labels.SortedBranches(targets)
			Expect(branches).To(Equal([]string{"release/v0.24", "release/v0.25", "release/v0.26"}))
		})
	})

	Describe("NormalizeBranch", func() {
		It("strips refs/heads prefix, whitespace, and surrounding slashes", func() {
			branch := labels.NormalizeBranch(" /refs/heads/release/v0.31/ ")
			Expect(branch).To(Equal("release/v0.31"))
		})

		It("returns empty string when normalization removes all characters", func() {
			Expect(labels.NormalizeBranch(" // ")).To(BeEmpty())
		})
	})
})
