# Cherry-Pick Action

> ⚠️ **ALPHA SOFTWARE**: This action is in active development and is not production-ready. APIs, behavior, and configuration may change without notice. Use at your own risk.

Automate cherry-picking merged pull requests to release branches with labels.

---

## Overview

This GitHub Action automates the tedious process of backporting changes to release branches. Simply add a label to any merged PR, and the action handles the rest.

**How it works:**
1. Merge your PR to the main branch
2. Add a label like `cherry-pick/release-v2.8` (works on PRs merged weeks ago too!)
3. Action creates a cherry-pick branch and opens a PR to the release branch
4. If conflicts occur, the workflow fails (or creates a placeholder PR if configured)

**Built for production teams:**
- Handles merge commits automatically
- Won't create duplicate PRs
- Clear failure visibility
- Works with GitHub Enterprise

---

## Core Features

- ✅ **Label-driven automation** - Just add `cherry-pick/<branch>` labels
- ✅ **Merge commit support** - Handles PRs merged with `--no-ff` or GitHub's merge button
- ✅ **Multiple targets** - Cherry-pick to several branches at once
- ✅ **Idempotent** - Safe to re-run, prevents duplicates automatically
- ✅ **Workflow failures** - Action fails when cherry-picks fail for clear visibility
- ✅ **Flexible branch names** - Supports slashes in branch names (e.g., `release/v2.9/security`)
- ✅ **Conflict strategies** - Choose to fail immediately or create placeholder PRs
- ✅ **GitHub Enterprise** - Full GHE support
- ✅ **GPG signing** - Optional commit signing
- ✅ **Dry run mode** - Test without creating real PRs

---

## Basic Usage

### 1. Add Workflow to Your Repository

Create `.github/workflows/cherry-pick.yml`:

```yaml
name: Cherry-Pick Automation

on:
  pull_request:
    types: [closed, labeled]
  pull_request_target:
    types: [closed, labeled]

permissions:
  contents: write
  pull-requests: write

jobs:
  cherry-pick:
    runs-on: ubuntu-latest
    if: github.event.pull_request.merged == true
    steps:
      - uses: rancher/cherry-pick-action@v1
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
```

### 2. Use Labels to Trigger Cherry-Picks

Label format: `cherry-pick/<branch-name>`

**Examples:**
- `cherry-pick/release-v2.8` → Cherry-picks to `release-v2.8` branch
- `cherry-pick/release/v2.7` → Supports slashes in branch names
- `cherry-pick/stable` → Any branch name works

**Multiple targets:**
Add multiple labels to cherry-pick to multiple branches:
- `cherry-pick/release-v2.8`
- `cherry-pick/release-v2.7`
- `cherry-pick/release-v2.6`

### 3. What Happens

When you add a label to a merged PR:

1. ✅ Action detects the label and target branch
2. ✅ Creates branch: `cherry-pick/release-v2.8/pr-123`
3. ✅ Cherry-picks the commits (handles merge commits automatically)
4. ✅ Opens new PR against `release-v2.8`
5. ✅ Adds `cherry-pick/done/release-v2.8` label to original PR
6. ✅ Posts comment with status

**If it fails:**
- ❌ Workflow fails with error message
- 📝 Comment posted on original PR explaining the failure
- 🔍 Check workflow logs for details

---

## Advanced Usage

### Configuration Options

All inputs are optional:

| Input | Description | Default |
|-------|-------------|---------|
| `github_token` | GitHub token with `contents:write` and `pull-requests:write` | `${{ github.token }}` |
| `label_prefix` | Prefix for cherry-pick labels | `cherry-pick/` |
| `conflict_strategy` | How to handle conflicts: `fail` or `placeholder-pr` | `fail` |
| `target_branches` | Override labels with comma/newline-separated branches | `""` |
| `dry_run` | Test mode - don't create PRs | `false` |
| `log_level` | Logging level: `debug`, `info`, `warn`, `error` | `info` |
| `github_base_url` | GitHub Enterprise API URL | `""` |
| `git_signing_key` | GPG private key for commit signing | `""` |
| `require_org_membership` | Skip if user is not org member | `false` |

### Handle Conflicts with Placeholder PRs

Instead of failing on conflicts, create a PR with an empty placeholder commit:

```yaml
- uses: rancher/cherry-pick-action@v1
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    conflict_strategy: placeholder-pr
```

The placeholder PR will have instructions for manual resolution.

### Specify Target Branches Without Labels

Cherry-pick to specific branches without using labels:

```yaml
- uses: rancher/cherry-pick-action@v1
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    target_branches: |
      release-v2.8
      release-v2.7
      release-v2.6
```

This overrides any `cherry-pick/*` labels on the PR.

### Test with Dry Run

Preview what would happen without creating real PRs:

```yaml
- uses: rancher/cherry-pick-action@v1
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    dry_run: true
    log_level: debug
```

### GitHub Enterprise Setup

```yaml
- uses: rancher/cherry-pick-action@v1
  with:
    github_token: ${{ secrets.GHE_TOKEN }}
    github_base_url: https://github.example.com/api/v3
```

### GPG Commit Signing

```yaml
- uses: rancher/cherry-pick-action@v1
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    git_signing_key: ${{ secrets.GPG_PRIVATE_KEY }}
    git_signing_passphrase: ${{ secrets.GPG_PASSPHRASE }}
```

The GPG key should be base64-encoded or ASCII-armored.

### Require Organization Membership

Prevent cherry-picks from external contributors:

```yaml
- uses: rancher/cherry-pick-action@v1
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    require_org_membership: true
```

Action will silently skip if the user triggering it is not a member of the repository owner organization.

### Use with Protected Branches

For protected release branches, use a Personal Access Token (PAT) or GitHub App token with bypass permissions:

```yaml
- uses: rancher/cherry-pick-action@v1
  with:
    github_token: ${{ secrets.CHERRY_PICK_PAT }}
```

The default `GITHUB_TOKEN` cannot push to protected branches.

---

## Q&A

### General Questions

**Q: Can I add cherry-pick labels to PRs that were merged weeks ago?**  
A: Yes! The action works on any merged PR, regardless of when it was merged. The workflow triggers on the `labeled` event.

**Q: What happens if I add the same label twice?**  
A: Nothing. The action is idempotent - it detects the existing `cherry-pick/done/<branch>` label and skips creation. The workflow succeeds (doesn't fail).

**Q: Can I cherry-pick to multiple branches at once?**  
A: Yes! Add multiple `cherry-pick/<branch>` labels, and the action creates separate PRs for each.

**Q: What if the target branch doesn't exist?**  
A: The action skips that target and posts a comment. The workflow succeeds for other targets if present.

**Q: Does this work with forked PRs?**  
A: The action skips PRs from forks by default. Create a branch in the base repository first.

### Merge Commits and Strategies

**Q: Does this handle merge commits?**  
A: Yes! The action automatically detects merge commits and uses `git cherry-pick -m 1` (first parent as mainline). This works for PRs merged with GitHub's "Create a merge commit" option or `git merge --no-ff`.

**Q: Does this work with squash merges?**  
A: Yes! For squash merges, the action cherry-picks the squashed commit.

**Q: What if I need a different merge parent than parent 1?**  
A: Currently only `-m 1` is supported. Manual cherry-picking is required for other parents.

### Conflicts and Failures

**Q: What happens when there's a conflict?**  
A: With `conflict_strategy: fail` (default), the workflow fails with an error. With `conflict_strategy: placeholder-pr`, it creates a PR with an empty commit and instructions for manual resolution.

**Q: Why did my workflow fail?**  
A: The workflow intentionally fails when cherry-picks fail to make issues visible. Check the workflow logs and PR comment for details. This ensures failures don't go unnoticed.

**Q: Does the workflow fail if a PR already exists?**  
A: No! Existing PRs are detected automatically and the workflow succeeds. Only actual failures (conflicts, errors) cause workflow failures.

**Q: How do I retry a failed cherry-pick?**  
A: Remove the `cherry-pick/done/<branch>` label (if it exists), then re-add the `cherry-pick/<branch>` label.

### Configuration and Customization

**Q: Can I change the label prefix from `cherry-pick/`?**  
A: Yes! Set `label_prefix: backport/` (or any prefix you want).

**Q: Can I customize the cherry-pick branch names?**  
A: Not currently. Branches follow the pattern `cherry-pick/<target-branch>/pr-<number>`.

**Q: How do I test without creating real PRs?**  
A: Set `dry_run: true` and check the logs to see what would happen.

**Q: What are the `cherry-pick/done/<branch>` labels?**  
A: These are added automatically after successful cherry-picks to prevent duplicates. They provide idempotency.

### Tokens and Permissions

**Q: Can I use the default `GITHUB_TOKEN`?**  
A: Yes, but it cannot push to protected branches. Use a PAT or GitHub App token for protected branches.

**Q: What permissions does the token need?**  
A: `contents: write` (to create branches and push) and `pull-requests: write` (to create PRs and add labels).

**Q: Will PRs created by `GITHUB_TOKEN` trigger other workflows?**  
A: No. Use a PAT or GitHub App token if you need to trigger other workflows.

### Troubleshooting

**Q: The action doesn't trigger when I add labels.**  
A: Check that your workflow file is on the default branch and includes `types: [closed, labeled]` in the event triggers.

**Q: I'm getting permission denied errors.**  
A: Verify the token has `contents: write` and `pull-requests: write` permissions. For protected branches, use a token with bypass rights.

**Q: Cherry-picks work locally but fail in the action.**  
A: Enable debug logging with `log_level: debug` and compare. Check for branch protection, missing branches, or environment differences.

---

## Additional Resources

- 📖 [Design Documentation](docs/design.md) - Architecture and implementation details
- 🧪 [Testing Guide](docs/TESTING_FROM_FORK.md) - How to test from your fork
- 📝 [Examples](examples/) - More workflow examples
- 🐛 [Report Issues](https://github.com/rancher/cherry-pick-action/issues)

---

## License

Apache 2.0 - See [LICENSE](LICENSE) file for details.
