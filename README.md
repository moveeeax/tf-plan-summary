# tf-plan-summary

> Turn noisy `terraform plan` output into a clean, reviewable PR comment.

**Status:** 🚧 In development

## Overview

GitHub Action that posts a human-readable summary of a Terraform plan (created/changed/destroyed) as a PR comment.

## Features

- Parses `terraform show -json plan.bin` into resource actions
- Groups by create / update / replace / destroy with counts
- Renders a collapsible Markdown table and sticky PR comment (updates in place)
- Highlights destroys and replacements as risk callouts
- Zero-config action: point it at the plan JSON artifact

## Stack

Go (plan JSON parser) + composite GitHub Action; go-github for PR comments.

## Usage

```bash
- uses: moveeeax/tf-plan-summary@v1
  with:
    plan-json: plan.json
    github-token: ${{ secrets.GITHUB_TOKEN }}
```

## License

MIT
