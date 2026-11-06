# tf-plan-summary

[![ci](https://github.com/moveeeax/tf-plan-summary/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/tf-plan-summary/actions/workflows/ci.yml)
[![go](https://img.shields.io/badge/go-1.26%2B-00ADD8)](https://go.dev)
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
| Action | Resource | Type | Why |
| + create | aws_s3_bucket.assets | aws_s3_bucket | |
| ± replace | aws_instance.web | aws_instance | forces replacement: ami |
| - destroy | aws_db_instance.legacy | aws_db_instance | removed from configuration |
</details>
```

## What it does

- Parses `terraform show -json plan.bin` into per-resource actions, across nested
  modules and `count`/`for_each` instances.
- Classifies each as **create / update / replace / destroy / forget** (a
  create+delete pair, in either order — the default and the `create_before_destroy`
  ordering — is a replacement), ignoring `no-op` and `read`.
- Renders counts, a collapsible details table, and a **risk callout** whenever the
  plan replaces, destroys, or forgets anything.
- Says **why** each replace and destroy happens, in a "Why" column: the attribute
  paths that force a replacement (`forces replacement: engine_version, identifier`)
  or Terraform's reason code (`tainted`, `replace requested (-replace)`,
  `removed from configuration`, `deposed object`, ...).
- Emits machine-readable counts (`--stats-json`, `$GITHUB_OUTPUT`) so a workflow can
  gate on `has-risk` instead of grepping Markdown.
- Applies a **risk policy**: `--protect` marks resources whose loss is critical,
  `--ignore-risk` exempts ones whose recreation nobody cares about, and
  `--fail-on` turns either into an exit code. See [Risk policy](#risk-policy).

It is deliberately conservative about under-reporting:

- Only `address`, `type`, `actions`, `action_reason`, `deposed` and the attribute
  paths in `replace_paths` are read from the plan. Values, `before`/`after` and
  `sensitive_values` are never parsed, so nothing from them can end up in a PR
  comment.
- An actions array it does not recognise is reported as `unclassified` rather than
  silently treated as a no-op.
- Input that is not a plan document (a state file from `terraform show -json`, a
  truncated download, an unrelated JSON file) is an **error**, never a reassuring
  "No changes".
- Resource addresses are escaped before they go into the Markdown table, so a
  `for_each` key containing pipes, backticks or newlines cannot break the table or
  hide the rows below it.
- A plan too large for GitHub's 65536-character comment limit has its table
  truncated — destroys and replacements first, everything else after — and the
  omission is stated in the comment. The counts are always complete.

## Use it as a GitHub Action

```yaml
permissions:
  contents: read
  pull-requests: write

steps:
  - uses: actions/checkout@v7
  - uses: hashicorp/setup-terraform@v4
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

### Action inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `plan-json` | yes | — | Path to `terraform show -json` output |
| `github-token` | yes | — | Token used to post the comment |
| `marker` | no | `<!-- tf-plan-summary -->` | Hidden marker identifying the sticky comment |
| `pr-number` | no | the triggering PR | Set explicitly on non-`pull_request` triggers |
| `protect` | no | — | Protected resources, one glob per line |
| `ignore-risk` | no | — | Resources exempt from `has-risk`, one glob per line |
| `fail-on` | no | `never` | Fail the step on `risk`, `critical`, or `never` |
| `version` | no | `latest` | Git ref of the CLI to install |
| `go-version` | no | `stable` | Go toolchain used to install the CLI |

### Action outputs

| Output | Example | Description |
|--------|---------|-------------|
| `create` / `update` / `replace` / `destroy` / `forget` | `3` | Per-action resource counts |
| `unknown` | `0` | Changes this version could not classify |
| `total` | `7` | All changes except `no-op` and `read` |
| `has-risk` | `true` | Anything replaced, destroyed, or forgotten, minus `ignore-risk` exemptions |
| `protected-destroy` / `protected-replace` | `1` | Protected resources destroyed / replaced |
| `has-critical-risk` | `true` | A protected resource is destroyed or replaced |
| `summary-file` | `/home/runner/work/_temp/tf-plan-summary.md` | The rendered Markdown |

Gate a job on the plan instead of eyeballing it:

```yaml
- uses: moveeeax/tf-plan-summary@v1
  id: summary
  with:
    plan-json: plan.json
    github-token: ${{ secrets.GITHUB_TOKEN }}

- name: Require approval for destructive plans
  if: steps.summary.outputs.has-risk == 'true'
  run: |
    echo "Plan destroys ${{ steps.summary.outputs.destroy }} and replaces ${{ steps.summary.outputs.replace }} resources."
    exit 1
```

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
| `--stats-json` | — | Write machine-readable counts to this JSON file |
| `--github-output` | `$GITHUB_OUTPUT` | Append `key=value` step outputs to this file |
| `--marker` | `<!-- tf-plan-summary -->` | Hidden marker identifying the sticky comment |
| `--github-token` | `$GITHUB_TOKEN` | Token for posting the comment |
| `--repo` | `$GITHUB_REPOSITORY` | `owner/name` of the repository |
| `--pr` | `$PR_NUMBER` | Pull request number |
| `--protect` | — | Glob of protected resources (repeatable) |
| `--ignore-risk` | — | Glob of resources exempt from `has-risk` (repeatable) |
| `--fail-on` | `never` | Exit with status 2 on `risk`, `critical`, or `never` |

Comment posting happens only when token, repo, and PR number are all set;
otherwise it just renders the Markdown.

## Risk policy

`has-risk` fires on any replacement, so a recreated `null_resource` blocks a PR just
like a destroyed database would. A policy tells the two apart:

```yaml
- uses: moveeeax/tf-plan-summary@v1
  with:
    plan-json: plan.json
    github-token: ${{ secrets.GITHUB_TOKEN }}
    protect: |
      aws_db_instance
      aws_s3_bucket.state
      module.data.*
    ignore-risk: |
      null_resource
      terraform_data
    fail-on: critical
```

Patterns are globs: `*` matches any run of characters (dots and brackets included),
`?` matches one character, and everything else is literal, so `aws_instance.web[0]`
needs no escaping. A pattern applies when it matches the full address
(`module.db.aws_db_instance.main`), the resource type (`aws_db_instance`), or the
address inside its module (`aws_db_instance.main`, which `aws_db_instance.*`
matches in any module).

- Protected rows are pinned to the top of the table with a 🔒, and a
  `[!CAUTION]` callout names every protected resource that is destroyed or
  replaced. `protected-destroy`, `protected-replace` and `has-critical-risk`
  count them.
- `--ignore-risk` takes a replacement, destroy or forget out of `has-risk` and
  the "Review carefully" callout. The change is still counted, listed with
  `(exempt)`, and the comment says how many were exempted.
- When a resource matches both, `--protect` wins.
- `--fail-on risk` fails on `has-risk`, `--fail-on critical` on
  `has-critical-risk`. The exit status is 2, distinct from 1 for a real error, and
  it comes after the comment, the step outputs and `--stats-json` are written.

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
