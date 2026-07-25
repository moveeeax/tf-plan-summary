# examples

- [`workflow.yml`](workflow.yml) — a PR workflow that runs `terraform plan`, converts it
  to JSON, and posts the summary as a sticky comment.
- [`plan.json`](plan.json) — a sample `terraform show -json` output to try locally.

Render it without GitHub:

```bash
tf-plan-summary --plan-json plan.json
```

Post/refresh a sticky comment on a real PR:

```bash
tf-plan-summary --plan-json plan.json \
  --repo owner/name --pr 42 --github-token "$GITHUB_TOKEN"
```

Get the counts as JSON instead of Markdown, for a gate of your own:

```bash
tf-plan-summary --plan-json plan.json -o /dev/null --stats-json stats.json
jq '.has_risk, .destroy' stats.json
```
