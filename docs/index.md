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
| `filter` | Filter services by official name, alias, or substring | `""` |
| `group-by` | Grouping dimension (`SERVICE`, `LINKED_ACCOUNT`, `TAG:<Key>`) | `SERVICE` |
| `name` | Human-readable name or alias for the AWS account (e.g. `Production Core`) | `""` |
| `tag` | Filter costs by AWS Cost Allocation Tag (e.g. `Environment=Production`) | `""` |
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

---

## CLI Usage & Commands

Running `aws-cost-report` without arguments prints help instantly (zero AWS authentication pause).

```bash
# Display instant CLI help
aws-cost-report

# Generate Month-over-Month report
aws-cost-report generate --compare

# Label report with a human-readable AWS account name
aws-cost-report generate --compare --name="Production Core"

# List official AWS Cost Explorer service names
aws-cost-report services

# Filter by official service names, aliases, or substrings
aws-cost-report generate --filter="Amazon DynamoDB,AWS Lambda"
aws-cost-report generate --filter="S3,EC2"

# Filter by Cost Allocation Tag (single tag, multi-value, or multiple tags)
aws-cost-report generate --tag="Environment=Production"
aws-cost-report generate --tag="Environment=Production,Staging" --compare
aws-cost-report generate --tag="Environment=Production" --tag="Project=FinOps"

# Combine tag filtering with account or secondary tag grouping
aws-cost-report generate --tag="Environment=Production" --group-by="LINKED_ACCOUNT"
aws-cost-report generate --tag="Environment=Production" --group-by="TAG:Owner"

# Identify untagged resource costs for a specific tag
aws-cost-report generate --group-by="TAG:Environment" --filter="Untagged" --compare
```

---

## How to Identify Untagged Costs

When grouping costs by an AWS Cost Allocation Tag dimension, AWS Cost Explorer aggregates all resources missing that tag into an empty tag key value. `aws-cost-report` automatically detects this and maps it to **`Untagged / Other`**.

### 1. View Breakdown of All Tag Values (Including Untagged Spend)

```bash
aws-cost-report generate --group-by="TAG:Environment"
```

In the generated report table, you will see a line item named `Untagged / Other` reflecting all costs incurred by resources lacking the `Environment` tag.

### 2. Isolate Only the Untagged Costs

Use the `--filter` option to match the untagged line item:

```bash
aws-cost-report generate --group-by="TAG:Environment" --filter="Untagged"
```

The report totals dynamically recalculate to show the exact total spend attributed to untagged resources.

### 3. Track Untagged Cost Trends Month-over-Month

Add `--compare` to see whether untagged spend is increasing or decreasing compared to the previous period:

```bash
aws-cost-report generate --group-by="TAG:Environment" --filter="Untagged" --compare
```

### Key Considerations

1. **Tag Activation**: The tag key (e.g. `Environment`, `Owner`, `Project`) must be activated as an active **Cost Allocation Tag** in the AWS Billing and Cost Management console before Cost Explorer can track or group by it.
2. **Evaluated Per Tag Key**: AWS Cost Explorer does not have a global "completely untagged" dimension across all potential tags simultaneously; grouping evaluates untagged resources for the specific tag key requested (e.g., `TAG:Environment`, `TAG:CostCenter`).
3. **Cost Aggregation vs. Resource IDs**: The tool reports aggregate monetary figures (e.g., `$450.00` on `Untagged / Other`), but does not enumerate individual resource ARNs or IDs (which would require AWS Config or the AWS Resource Groups Tagging API).
