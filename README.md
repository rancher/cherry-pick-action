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
      contents: write

    # For now, only run if the PR is merged
    if: github.event.pull_request.merged == true && startsWith(github.event.label.name, 'cherry-pick/')
    runs-on: ubuntu-latest
    steps:
    - run: |
        git config --global user.name "github-actions[bot]"
        git config --global user.email "41898282+github-actions[bot]@users.noreply.github.com"

    - uses: rancher/cherry-pick-action/from-label@main
      with:
        token: ${{ secrets.GITHUB_TOKEN }}
        label-added: ${{ github.event.label.name }}
        all-labels-json: ${{ toJSON(github.event.pull_request.labels.*.name) }}
        pull-request: ${{ github.event.pull_request.number }}
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
