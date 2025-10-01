# Cherry-pick GHA

# How to

## Using labels

In your repository, add the following two workflows.

1. Add the workflow that will trigger on label on merged pull requests in `.github/workflows/backport-label.yaml`

```yaml
name: Backport label

on:
  pull_request:
    types: [labeled]
    branches:
    - main
    - 'release/**'

jobs:
  backport-label:
    permissions:
      # Necessary to add cherry-pick-done/* label
      pull-requests: write
      # Necessary to create the PR
      #
      # NOTE: We'll want to switch to an app token once we have one.
      # Reason: GHA is designed to not trigger other workflows when creating a PR
      # within a workflow run using the workflow run token.
      #
      # Temporary workaround is to close/re-open the PR in the UI. This will trigger
      # the pull_request event.
      contents: write

    # For now, only run if the PR is merged
    if: github.event.pull_request.merged == true && startsWith(github.event.label.name, 'cherry-pick/')
    uses: rancher/cherry-pick-action/.github/workflows/cherry-pick-from-labels.yaml@main
    with:
      # Must match the version used for the reusable workflow above (eg: @<ref>)
      workflow-ref: main
      label-added: ${{ github.event.label.name }}
      all-labels-json: ${{ toJSON(github.event.pull_request.labels.*.name) }}
      pr-number: ${{ github.event.pull_request.number }}
    secrets:
      token: ${{ secrets.GITHUB_TOKEN }}
```

2. Add the workflow that will automatically generate labels for your release
   branches at `.github/workflows/generate-cherry-pick-labels.yaml`

```yaml
name: Auto-generate cherry-pick labels

on:
  workflow_dispatch:
  create:

jobs:
  generate-cherry-pick-labels:
    permissions:
      # Necessary to create/edit labels
      issues: write
    uses: rancher/cherry-pick-action/.github/workflows/generate-cherry-pick-labels.yaml@main
    with:
      branch-filter-regex: "^main|release/.*"
    secrets:
      token: ${{ secrets.GITHUB_TOKEN }}
```
