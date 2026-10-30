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
        uses: smford/aws-cost-report@latest # or @v0.7.0
        with:
          compare: 'true'
          threshold: '500.00'
          format: 'markdown'
```

### Action Inputs & Outputs

| Input | Description | Default |
|:---|:---|:---:|
| `version` | Target binary release version (e.g. `v0.7.0` or `latest`) | `latest` |
| `compare` | Enable Month-over-Month comparison | `true` |
| `format` | Output format (`markdown`, `table`, `json`, `csv`) | `markdown` |
| `filter` | Filter services by official name, alias, or substring | `""` |
| `group-by` | Grouping dimension(s), up to 2 comma-separated (`SERVICE`, `LINKED_ACCOUNT`, `TAG:<Key>`) | `SERVICE` |
| `name` | Human-readable name or alias for the AWS account (e.g. `Production Core`) | `""` |
| `tag` | Filter costs by AWS Cost Allocation Tag (e.g. `Environment=Production`) | `""` |
| `exclude-credits` | Exclude promotional credits and refunds (`true`/`false`) | `false` |
| `charge-type` | Filter costs by charge/record type (e.g. `Usage,Tax`, `Credit`) | `""` |
| `threshold` | Budget threshold dollar amount | `""` |
| `fail-on-threshold` | Fail step if budget threshold exceeded | `true` |
| `top` | Show top N highest cost items and aggregate remainder into Other | `""` |
| `slack-webhook` | Incoming Slack webhook URL for automated ChatOps | `""` |
| `teams-webhook` | Incoming Microsoft Teams webhook URL for automated ChatOps | `""` |
| `write-summary` | Write report to `$GITHUB_STEP_SUMMARY` | `true` |
| `monthly` | Render multi-month historical breakdown in column format | `false` |
| `daily` | Query daily spend granularity over the billing period | `false` |
| `forecast` | Query AWS Cost Explorer projected month-end spend forecast and guardrail | `false` |
| `mock` | Run offline with synthetic dataset | `false` |
| `output-file` | Optional path to save output file or `s3://` URI | `""` |
| `s3-bucket` | Target AWS S3 bucket name for saving report output | `""` |
| `s3-key` | Optional S3 object key prefix/path | `""` |

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

# Generate Month-over-Month report (auto-resolves IAM account alias if configured)
aws-cost-report generate --compare

# Explicitly override report with a custom human-readable account name
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

# Two-Dimensional Grouping (combine up to 2 dimensions)
aws-cost-report generate --group-by="SERVICE,TAG:Environment"
aws-cost-report generate --group-by="LINKED_ACCOUNT,SERVICE"
aws-cost-report generate --group-by="TAG:Environment,TAG:Team"

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

---

## Multi-Month Billing & 12-Month Column Trend

View historical costs across multiple months side-by-side in dedicated columns:

```bash
# View the previous 12 months in column format (default: 12 months)
aws-cost-report generate --months=12 --monthly

# View the previous 6 months
aws-cost-report generate --months=6 --monthly

# Export 12-month pivot table to CSV for spreadsheets
aws-cost-report generate --months=12 --monthly --format=csv -o 12-months.csv
```

---

## Month-to-Date (MTD) Spend Tracking

Query real-time in-progress spend for the active month up to today:

```bash
# Query Month-to-Date spend for current billing cycle
aws-cost-report generate --mtd

# Compare current MTD spend against the same day window last month
aws-cost-report generate --mtd --compare
```

---

## Daily Spend Granularity (`--daily`)

Analyze day-by-day spending patterns across your billing window with daily totals, day-of-week context, daily averages, and the top cost driver for each day:

```bash
# Query daily spend breakdown for current month:
aws-cost-report generate --daily

# Combine daily granularity with Month-to-Date (MTD) tracking:
aws-cost-report generate --daily --mtd

# Export daily breakdown to CSV:
aws-cost-report generate --daily --format=csv -o daily-spend.csv
```

---

## Predictive Budget Forecast Guardrail (`--forecast`)

Use AWS Cost Explorer's machine learning engine to project total month-end spend and trigger budget alerts before overruns occur:

```bash
# Query projected month-end spend forecast:
aws-cost-report generate --forecast

# Combine with budget threshold to alert or fail if projected spend breaches limit:
aws-cost-report generate --forecast --threshold=500.00
```

---

## Prometheus Metrics Exporter & Daemon

Expose real-time AWS cost metrics in standard Prometheus exposition format (`version 0.0.4`) for Prometheus and Grafana:

```bash
# Start HTTP daemon on port 8080
aws-cost-report --server

# Scrape Prometheus metrics
curl http://localhost:8080/metrics

# Or export metrics directly via CLI
aws-cost-report generate --format=prometheus
```

---

## Shell Autocompletion

Generate dynamic autocompletion scripts for subcommands, flags, dimensions, and formats:

```bash
# Bash:
source <(aws-cost-report completion bash)

# Zsh:
source <(aws-cost-report completion zsh)

# Fish:
aws-cost-report completion fish | source
```

---

## Exporting Reports to Amazon S3

Save cost reports or machine-readable JSON summaries directly to an S3 bucket:

```bash
# Upload report markdown directly to S3
aws-cost-report generate --compare --output="s3://my-billing-bucket/reports/2026-10.md"

# Upload both formatted report and JSON summary to S3
aws-cost-report generate --compare \
  --output="s3://my-billing-bucket/cost-report.md" \
  --json-summary="s3://my-billing-bucket/cost-summary.json"

# Specify target bucket with relative output path
aws-cost-report generate --output="monthly-report.csv" --format=csv --s3-bucket="my-billing-bucket"
```

---

## Enterprise Discount Program (EDP) & Net Cost Metrics

For accounts with AWS Enterprise Discount Program (EDP) discounts or private pricing agreements, query net metrics:

```bash
# Query net discount metrics alongside standard costs:
aws-cost-report generate --compare --metrics="UnblendedCost,NetUnblendedCost,NetAmortizedCost"

# Or query all supported metrics at once:
aws-cost-report generate --compare --metrics=all
```
