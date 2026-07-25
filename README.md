# tf-plan-summary

[![ci](https://github.com/moveeeax/tf-plan-summary/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/tf-plan-summary/actions/workflows/ci.yml)
[![go](https://img.shields.io/badge/go-1.22%2B-00ADD8)](https://go.dev)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

> Turn noisy `terraform plan` output into a clean, reviewable PR comment.

`tf-plan-summary` reads a `terraform show -json` plan, groups the resource changes
by action, and renders a compact Markdown summary. As a GitHub Action it posts one
**sticky** comment that refreshes in place on every push — so a PR shows the current
plan, not a pile of stale ones.

```
## Terraform Plan Summary

**2** to add, **1** to change, **1** to replace, **1** to destroy.

> ⚠️ Review carefully: 1 to replace, 1 to destroy.

<details><summary>Details (5 resources)</summary>
| Action | Resource | Type |
| + create | aws_s3_bucket.assets | aws_s3_bucket |
| ± replace | aws_instance.web | aws_instance |
| - destroy | aws_db_instance.legacy | aws_db_instance |
</details>
```

## What it does

- Parses `terraform show -json plan.bin` into per-resource actions.
- Classifies each as **create / update / replace / destroy** (a create+delete pair,
  in either order, is a replacement), ignoring `no-op` and `read`.
- Renders counts, a collapsible details table, and a **risk callout** whenever the
  plan replaces or destroys anything.
- Exit code stays `0` for a clean render; wire the threshold gate in CI as you like.

## Use it as a GitHub Action

```yaml
permissions:
  contents: read
  pull-requests: write

steps:
  - uses: actions/checkout@v4
  - uses: hashicorp/setup-terraform@v3
  - run: |
      terraform init -input=false
      terraform plan -out=plan.bin -input=false
      terraform show -json plan.bin > plan.json
  - uses: moveeeax/tf-plan-summary@v1
    with:
      plan-json: plan.json
      github-token: ${{ secrets.GITHUB_TOKEN }}
```

The action finds its previous comment by a hidden marker and edits it, so re-runs
never spam the thread. See [`examples/`](examples/) for a full workflow.

## Use it as a CLI

```bash
go install github.com/moveeeax/tf-plan-summary@latest

# render to stdout
tf-plan-summary --plan-json plan.json

# post/refresh a sticky comment
tf-plan-summary --plan-json plan.json --repo owner/name --pr 42 --github-token "$GITHUB_TOKEN"
```

| Flag | Default | Description |
|------|---------|-------------|
| `--plan-json` | — | Path to `terraform show -json` output (required) |
| `-o`, `--output` | `-` | Write Markdown to a file instead of stdout |
| `--marker` | `<!-- tf-plan-summary -->` | Hidden marker identifying the sticky comment |
| `--github-token` | `$GITHUB_TOKEN` | Token for posting the comment |
| `--repo` | `$GITHUB_REPOSITORY` | `owner/name` of the repository |
| `--pr` | `$PR_NUMBER` | Pull request number |

Comment posting happens only when token, repo, and PR number are all set;
otherwise it just renders the Markdown.

## How it works

The plan parser reads `resource_changes[].change.actions` and collapses the array
into one effective action — the only subtlety is that Terraform expresses a
replacement as `["create","delete"]` (or the reverse), which is detected explicitly.
The sticky-comment logic sits behind a small `Commenter` interface, so the
create-vs-update decision is unit-tested without touching the GitHub API; the real
implementation uses [`go-github`](https://github.com/google/go-github).

## Development

```bash
make build   # go build -o tf-plan-summary .
make test    # go test -race ./...
make lint    # gofmt -l . && go vet ./...
```

## License

MIT — see [LICENSE](LICENSE).
