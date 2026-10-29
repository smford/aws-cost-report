# aws-cost-report Documentation

[![CI](https://github.com/smford/aws-cost-report/actions/workflows/ci.yml/badge.svg)](https://github.com/smford/aws-cost-report/actions/workflows/ci.yml)
[![GitHub Release](https://img.shields.io/github/v/release/smford/aws-cost-report?logo=github)](https://github.com/smford/aws-cost-report/releases)
[![GitHub Action](https://img.shields.io/badge/GitHub%20Action-smford%2Faws--cost--report-blue?logo=githubactions&logoColor=white)](https://github.com/smford/aws-cost-report)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A production-grade, zero-dependency Go CLI tool, GitHub Action, and SRE budget guardrail for AWS Cost Explorer.

---

## GitHub Action Quickstart

`aws-cost-report` is available as a first-class GitHub Action that runs natively on Ubuntu and macOS runners.

```yaml
name: AWS Cost Guardrail

on:
  schedule:
    - cron: '0 9 1 * *' # Run at 09:00 UTC on the 1st of every month
  workflow_dispatch:

permissions:
  id-token: write
  contents: read

jobs:
  cost-report:
    runs-on: ubuntu-latest
    steps:
      - name: Configure AWS Credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@ececac1a45f3b08a01d2dd070d28d111c5fe6722 # v4.1.0
        with:
          role-to-assume: arn:aws:iam::123456789012:role/GitHubActionsCostReport
          aws-region: us-east-1

      - name: Generate Cost Report
        uses: smford/aws-cost-report@v0.1.0
        with:
          compare: 'true'
          threshold: '500.00'
          format: 'markdown'
```

### Action Inputs & Outputs

| Input | Description | Default |
|:---|:---|:---:|
| `version` | Target binary release version | `v0.1.0` |
| `compare` | Enable Month-over-Month comparison | `true` |
| `format` | Output format (`markdown`, `table`, `json`, `csv`) | `markdown` |
| `threshold` | Budget threshold dollar amount | `""` |
| `fail-on-threshold` | Fail step if budget threshold exceeded | `true` |
| `top` | Show top N highest cost items and aggregate remainder into Other | `""` |
| `slack-webhook` | Incoming Slack webhook URL for automated ChatOps | `""` |
| `write-summary` | Write report to `$GITHUB_STEP_SUMMARY` | `true` |
| `mock` | Run offline with synthetic dataset | `false` |
| `output-file` | Optional path to save output file | `""` |

Outputs include `report`, `total-cost`, `previous-cost`, `cost-delta`, `cost-delta-pct`, and `threshold-breached`.

---

## Installation

### Homebrew (macOS & Linux)
```bash
brew tap smford/homebrew-tap
brew install aws-cost-report
```

### Standalone Binary
Download pre-compiled binaries from [GitHub Releases](https://github.com/smford/aws-cost-report/releases).
