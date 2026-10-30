# aws-cost-report

[![CI](https://github.com/smford/aws-cost-report/actions/workflows/ci.yml/badge.svg)](https://github.com/smford/aws-cost-report/actions/workflows/ci.yml)
[![SAST](https://github.com/smford/aws-cost-report/actions/workflows/sast.yml/badge.svg)](https://github.com/smford/aws-cost-report/actions/workflows/sast.yml)
[![DAST](https://github.com/smford/aws-cost-report/actions/workflows/dast.yml/badge.svg)](https://github.com/smford/aws-cost-report/actions/workflows/dast.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/smford/aws-cost-report/badge)](https://scorecard.dev/viewer/?uri=github.com/smford/aws-cost-report)
[![GitHub Release](https://img.shields.io/github/v/release/smford/aws-cost-report?logo=github)](https://github.com/smford/aws-cost-report/releases)
[![GitHub Action](https://img.shields.io/badge/GitHub%20Action-smford%2Faws--cost--report-blue?logo=githubactions&logoColor=white)](https://github.com/smford/aws-cost-report)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Documentation](https://img.shields.io/badge/docs-GitHub%20Pages-blue.svg)](http://stephenford.org/aws-cost-report/)

A production-grade, zero-dependency (outside official AWS SDK v2 and YAML parser) Go CLI tool and automation utility for AWS Cost Explorer. Designed for developer workstations and headless execution inside GitHub Actions CI/CD pipelines.

---

## The Problem It Solves

- **Slow and Overwhelming AWS Console**: Navigating the AWS Billing Console to find monthly spend breakdown requires navigating complex UI filters, pagination, and heavy permissions.
- **Lack of Local SRE Visibility**: Developers and platform engineers cannot quickly inspect recent month-over-month infrastructure cost swings directly from their terminals.
- **No CI/CD Cost Guardrails**: Cloud pipelines often deploy infrastructure changes without automated guardrails to prevent accidental budget overruns or resource misconfigurations.
- **Headless Reporting Friction**: Formatting AWS Cost Explorer output into clean GitHub Actions summaries or machine-readable exports (JSON/CSV) typically requires writing brittle bash scripts or heavy Python runtimes.

`aws-cost-report` provides an instantaneous, statically-compiled binary that queries AWS Cost Explorer, computes Month-over-Month (MoM) deltas, evaluates budget guardrails, and renders terminal tables, GitHub Step Summaries, JSON, or CSV.

---

## Key Features

- **Strict AWS Date Boundaries**: Automatically handles AWS Cost Explorer's inclusive-Start / exclusive-End date rules while presenting human-readable inclusive dates in all reports.
- **Three Core Cost Dimensions**: Simultaneously extracts and reports `UnblendedCost`, `BlendedCost`, and `AmortizedCost`.
- **Month-over-Month (MoM) Delta**: Computes absolute dollar difference ($) and percentage changes (% Δ) against preceding billing periods with ANSI colorized delta indicators (green for reductions, red for increases).
- **Multi-Month Column Breakdown (`--monthly` / `--trend`)**: View up to 12+ historical months side-by-side in multi-column format (each month has its own column, plus total), in terminal tables, Markdown, CSV, and JSON.
- **Month-to-Date Tracking (`--mtd` / `--current-month`)**: Query real-time in-progress spend from the 1st of the current month up to today with automatic MoM comparison against the same date window last month.
- **Daily Spend Granularity (`--daily` / `--granularity=DAILY`)**: Track day-by-day cost progression across billing cycles with daily totals, day-of-week context, and top spending services per day.
- **Multi-Dimension Grouping**: Group by AWS `SERVICE`, `LINKED_ACCOUNT`, or custom Cost Allocation Tags (`TAG:Environment`, `TAG:Project`, etc.).
- **SRE Budget Guardrails (`--fail-on-threshold` / `--threshold`)**: Blocks CI/CD pipelines by terminating with exit code `1` if total billing exceeds a configured dollar threshold.
- **Predictive Forecast Guardrail (`--forecast`)**: Uses AWS Cost Explorer machine learning (`GetCostForecast`) to project month-end spend and warn or fail pipelines before overruns occur.
- **Multi-Format Output Engines**:
  - `table`: Clean fixed-width ASCII tables formatted like official AWS billing statements.
  - `markdown`: GitHub-flavored Markdown automatically written to `$GITHUB_STEP_SUMMARY` in CI/CD.
  - `json`: Structured JSON array for downstream analytics and log aggregators.
  - `csv`: Standard CSV format with column headers.
  - `prometheus`: Standard Prometheus text exposition format (version 0.0.4) for metrics scraping and dashboards.
- **Built-in Rate Limiting & Resilience**: Employs exponential backoff with jitter to absorb AWS Cost Explorer API rate limits (`LimitExceededException`).
- **Concurrent Comparison Fetching**: Fetches current and historical periods concurrently, reducing query latency by 50%.
- **Zero-AWS Offline Mode (`--mock`)**: Generates realistic simulated billing data for testing pipelines and local evaluations without active AWS credentials.

---

## Terminal Table Preview

```text
========================================================================================================
                                   AWS MONTHLY BILLING & USAGE REPORT
========================================================================================================
 Account ID:         715195480427
 Billing Month:      September 2026
 Period:             2026-09-01 to 2026-09-30 (inclusive)
 Currency:           USD
 Estimated / Final:  Finalized
 Active Services:    14
--------------------------------------------------------------------------------------------------------
 Metric                   |       Amount (USD) |        Prev (USD) |         Diff ($) |              % Δ
--------------------------------------------------------------------------------------------------------
 Unblended Cost           |              21.58 |             21.13 |            +0.45 |           +2.13%
 Blended Cost             |              21.59 |             21.14 |            +0.45 |           +2.13%
 Amortized Cost           |              21.58 |             21.13 |            +0.45 |           +2.13%
========================================================================================================

--------------------------------------------------------------------------------------------------------
 Service Name                               | Unblended (USD) |     Prev (USD) |   Diff ($) |        % Δ
--------------------------------------------------------------------------------------------------------
 Amazon DynamoDB                            |            5.51 |           5.20 |      +0.31 |     +5.96%
 AWS Config                                 |            4.73 |           4.80 |      -0.07 |     -1.46%
 Amazon Cognito                             |            4.17 |           4.15 |      +0.02 |     +0.48%
 Tax                                        |            2.89 |           2.80 |      +0.09 |     +3.21%
 AWS Key Management Service                 |            1.00 |           1.00 |       0.00 |      0.00%
 Amazon Simple Storage Service              |            0.94 |           0.88 |      +0.06 |     +6.82%
 Amazon Route 53                            |            0.80 |           0.80 |       0.00 |      0.00%
 Amazon CloudWatch                          |            0.65 |           0.70 |      -0.05 |     -7.14%
 AWS Secrets Manager                        |            0.40 |           0.40 |       0.00 |      0.00%
 AWS Lambda                                 |            0.25 |           0.18 |      +0.07 |    +38.89%
 Amazon Virtual Private Cloud               |            0.12 |           0.12 |       0.00 |      0.00%
 Amazon Simple Notification Service         |            0.06 |           0.05 |      +0.01 |    +20.00%
 AWS Step Functions                         |            0.04 |           0.03 |      +0.01 |    +33.33%
 AWS CloudTrail                             |            0.02 |           0.02 |       0.00 |      0.00%
========================================================================================================
```

---

## Installation

### Homebrew (macOS & Linux)

```bash
brew tap smford/homebrew-tap
brew install aws-cost-report
```

### Pre-Compiled Binaries

Download pre-compiled static binaries from the [GitHub Releases](https://github.com/smford/aws-cost-report/releases) page for:
- `darwin/arm64` (Apple Silicon)
- `darwin/amd64` (Intel Mac)
- `linux/arm64` (ARM64 / Graviton)
- `linux/amd64` (x86_64 Linux)

### Docker Container

```bash
docker pull ghcr.io/smford/aws-cost-report:latest
# or explicit version
docker pull ghcr.io/smford/aws-cost-report:v0.7.0

docker run --rm \
  -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_REGION=us-east-1 \
  ghcr.io/smford/aws-cost-report:latest
```

### Go Install

```bash
go install github.com/smford/aws-cost-report/cmd/aws-cost-report@latest
```

---

## Quickstart & Commands

Running `aws-cost-report` without arguments prints the help message instantly with zero network latency (no AWS authentication attempt or pause).

```bash
# Print instant CLI help and available options
aws-cost-report
```

### 1. Generate Report (Previous Complete Month)
To initiate a report, pass the `generate` command. By default, it queries the last completed calendar month:

```bash
aws-cost-report generate
```

### 2. Month-over-Month Comparison
Calculates absolute delta and percentage shift compared to the preceding period:

```bash
aws-cost-report generate --compare
```

### 3. Human-Readable AWS Account Labeling (`--name` & Auto-Discovery)
`aws-cost-report` displays a friendly human-readable name for your AWS account alongside the 12-digit AWS Account ID in report headers.

**Automatic Discovery**:
If `--name` and config `name:` are omitted, `aws-cost-report` automatically queries AWS IAM (`iam:ListAccountAliases`) to resolve your account's IAM Account Alias (e.g. `my-company-prod`). If the role lacks IAM permissions or no alias is set, it falls back cleanly to the numeric Account ID without error.

**Manual / Config Overrides**:
You can also explicitly supply or override the account name via CLI flag or configuration:

```bash
# Explicitly set account name via CLI
aws-cost-report generate --compare --name="Production Core"
```

In `config.yaml`, the account name can be permanently configured:

```yaml
name: "Production Core"
```

Report headers across all renderers (Table, Markdown, Slack, JSON) will clearly display the friendly alias:
```text
 Account ID:         123456789012 (Production Core)
```

### 4. List Official AWS Service Names
AWS Cost Explorer uses official service identifiers (such as `Amazon DynamoDB`, `AWS Lambda`, and `Amazon Simple Storage Service`). You can list all official names available in your account or environment:

```bash
# List all available official AWS Cost Explorer service names
aws-cost-report services

# In offline mock mode
aws-cost-report services --mock
```

### 5. Service Filtering & Aliases
Focus on specific workloads. You can filter using **official full names**, **common abbreviations**, or **case-insensitive substrings**:

```bash
# Using exact official AWS Cost Explorer names:
aws-cost-report generate --filter="Amazon DynamoDB,AWS Lambda"

# Using common aliases (S3, EC2, KMS, VPC, SQS, SNS, CloudWatch, etc.):
aws-cost-report generate --filter="S3,EC2,KMS"

# Using substrings:
aws-cost-report generate --filter="DynamoDB,Lambda"
```

### 6. Filtering by Cost Allocation Tags (`--tag` / `--tag-filter`)
Scope your entire billing report to specific workloads tagged in AWS. AWS Cost Explorer natively filters both totals and breakdown items to resources bearing the specified tags:

```bash
# Example A: Filter by single tag
aws-cost-report generate --tag="Environment=Production"

# Example B: Filter by tag with multiple allowed values (logical OR)
aws-cost-report generate --tag="Environment=Production,Staging"

# Example C: Filter by multiple different tags (logical AND) with Month-over-Month comparison
aws-cost-report generate --tag="Environment=Production" --tag="Project=FinOps" --compare

# Example D: Group tag-filtered costs by linked account
aws-cost-report generate --tag="Environment=Production" --group-by="LINKED_ACCOUNT"

# Example E: Group tag-filtered costs by a secondary tag dimension
aws-cost-report generate --tag="Environment=Production" --group-by="TAG:Owner"
```

In `config.yaml`, tags can also be permanently configured:

```yaml
tag_filters:
  Environment:
    - "Production"
    - "Staging"
  Project: "FinOps"
```

### 7. Multi-Dimensional Grouping (Single or Two-Dimensional)
Break down costs by environment, team, or AWS Organization linked account, or combine up to two dimensions simultaneously (supported natively by AWS Cost Explorer):

```bash
# Group by Cost Allocation Tag
aws-cost-report generate --group-by="TAG:Environment"

# Group by Linked Account (supports friendly name mapping in config.yaml)
aws-cost-report generate --group-by="LINKED_ACCOUNT"

# Two-Dimensional Grouping: Service + Tag (e.g. Amazon EC2 [Environment=Production])
aws-cost-report generate --group-by="SERVICE,TAG:Environment"

# Two-Dimensional Grouping: Linked Account + Service
aws-cost-report generate --group-by="LINKED_ACCOUNT,SERVICE"

# Two-Dimensional Grouping: Two Tags (Environment + Team)
aws-cost-report generate --group-by="TAG:Environment,TAG:Team"
```

### 8. How to Identify Untagged Costs
When you group costs by an AWS Cost Allocation Tag dimension, AWS Cost Explorer aggregates all resources missing that tag into an empty tag key value. `aws-cost-report` automatically detects this and maps it to **`Untagged / Other`**.

#### 1. View Breakdown of All Tag Values (Including Untagged Spend)
```bash
aws-cost-report generate --group-by="TAG:Environment"
```
In the generated report table, you will see a line item named `Untagged / Other` reflecting all costs incurred by resources lacking the `Environment` tag.

#### 2. Isolate Only the Untagged Costs
Use the `--filter` option to match the untagged line item:
```bash
aws-cost-report generate --group-by="TAG:Environment" --filter="Untagged"
```
The report totals dynamically recalculate to show the exact total spend attributed to untagged resources.

#### 3. Track Untagged Cost Trends Month-over-Month
Add `--compare` to see whether untagged spend is increasing or decreasing compared to the previous period:
```bash
aws-cost-report generate --group-by="TAG:Environment" --filter="Untagged" --compare
```

> [!NOTE]
> **Key Considerations for Untagged Cost Tracking**:
> 1. **Tag Activation**: The tag key (e.g. `Environment`, `Owner`, `Project`) must be activated as an active **Cost Allocation Tag** in the AWS Billing and Cost Management console before Cost Explorer can track or group by it.
> 2. **Evaluated Per Tag Key**: AWS Cost Explorer does not have a global "completely untagged" dimension across all potential tags simultaneously; grouping evaluates untagged resources for the specific tag key requested (e.g., `TAG:Environment`, `TAG:CostCenter`).
> 3. **Cost Aggregation vs. Resource IDs**: The tool reports aggregate monetary figures (e.g., `$450.00` on `Untagged / Other`), but does not enumerate individual resource ARNs or IDs (which would require AWS Config or the AWS Resource Groups Tagging API).

### 9. Multi-Month Trend & 12-Month Column Breakdown (`--monthly` / `--trend`)
View up to 12+ historical billing months in a multi-column pivot table where each month is rendered in its own dedicated column alongside cumulative totals:

```bash
# View the previous 12 months side-by-side in column format (default: 12 months)
aws-cost-report generate --months=12 --monthly

# View the previous 6 months
aws-cost-report generate --months=6 --monthly

# Export 12-month column pivot to CSV or Markdown
aws-cost-report generate --months=12 --monthly --format=csv -o 12-month-costs.csv
aws-cost-report generate --months=12 --monthly --format=markdown
```

#### Terminal Output Preview:
```text
===================================================================================================================================================================================
                                                               AWS MONTHLY BILLING & USAGE REPORT (HISTORICAL TREND)
===================================================================================================================================================================================
 Account ID:         715195480427 (Production Core)
 Billing Period:     October 2025 - September 2026 (12 Months)
 Period:             2025-10-01 to 2026-09-30 (inclusive)
 Currency:           USD
 Estimated / Final:  Finalized
 Active Services:    14
 Grand Total:        $258.97
-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 Service Name                     |   Oct 25 |   Nov 25 |   Dec 25 |   Jan 26 |   Feb 26 |   Mar 26 |   Apr 26 |   May 26 |   Jun 26 |   Jul 26 |   Aug 26 |   Sep 26 | Total (USD)
-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 Amazon DynamoDB                  |     5.29 |     5.68 |     5.57 |     5.45 |     5.34 |     5.73 |     5.62 |     5.51 |     5.40 |     5.29 |     5.68 |     5.57 |      66.13
 AWS Config                       |     4.78 |     4.68 |     4.59 |     4.92 |     4.82 |     4.73 |     4.64 |     4.54 |     4.87 |     4.78 |     4.68 |     4.59 |      56.62
 Amazon Cognito                   |     4.04 |     4.34 |     4.25 |     4.17 |     4.09 |     4.00 |     4.30 |     4.21 |     4.13 |     4.04 |     4.34 |     4.25 |      50.16
 Tax                              |     2.95 |     2.89 |     2.83 |     2.77 |     2.98 |     2.92 |     2.86 |     2.80 |     3.01 |     2.95 |     2.89 |     2.83 |      34.68
 AWS Key Management Service       |     0.98 |     0.96 |     1.03 |     1.01 |     0.99 |     0.97 |     1.04 |     1.02 |     1.00 |     0.98 |     0.96 |     1.03 |      11.97
 Amazon Simple Storage Service    |     0.97 |     0.95 |     0.93 |     0.91 |     0.98 |     0.96 |     0.94 |     0.92 |     0.90 |     0.97 |     0.95 |     0.93 |      11.31
...
-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
 Total (USD)                      |    21.36 |    21.83 |    21.55 |    21.59 |    21.50 |    21.66 |    21.72 |    21.37 |    21.65 |    21.36 |    21.83 |    21.55 |     258.97
===================================================================================================================================================================================
```

### 10. Month-to-Date (MTD) Spend Tracking (`--mtd` / `--current-month`)
Query real-time in-progress spend from the 1st of the current month up to today without needing to calculate or pass explicit dates:

```bash
# Query Month-to-Date spend for the current cycle
aws-cost-report generate --mtd

# Compare current MTD spend against the exact same day range last month
aws-cost-report generate --mtd --compare
```

When `--compare` is combined with `--mtd`, the previous period automatically matches the identical day-of-month window from the preceding month (e.g. October 1–29 vs. September 1–29).

### 11. Daily Spend Breakdown (`--daily` / `--granularity=DAILY`)
Analyze day-by-day spending patterns across your billing window with daily totals, day-of-week context, daily averages, and the top cost driver for each day:

```bash
# Query daily spend breakdown for current month:
aws-cost-report generate --daily

# Combine daily granularity with Month-to-Date (MTD) tracking:
aws-cost-report generate --daily --mtd

# Export daily breakdown to CSV:
aws-cost-report generate --daily --format=csv -o daily-spend.csv
```

### 12. Top N Cost Drivers
Highlight the largest cost drivers and consolidate all smaller workloads into `"Other Services"`:

```bash
aws-cost-report generate --compare --top=5
```

### 13. Explicit Date Range
Query custom date intervals (for custom ranges, comparison periods match the exact duration in days):

```bash
aws-cost-report generate --start=2026-08-01 --end=2026-09-30
```

### 14. SRE Budget Guardrail & Predictive Forecast (CI/CD Quality Gate)
Exit with status code `1` if total unblended cost exceeds threshold, or enable `--forecast` to alert when projected month-end spend is on track to breach your budget:

```bash
# Fail CI/CD step if current spend exceeds $50.00:
aws-cost-report generate --threshold=50.00
echo $? # Returns 1 if total cost > $50.00, or 0 if within budget

# Predictive Budget Guardrail: Fail CI/CD step if projected month-end spend breaches $50.00:
aws-cost-report generate --forecast --threshold=50.00
```

### 15. Export to HTML, JSON, or CSV

```bash
# Standalone, interactive HTML report with responsive dark/light dashboard
aws-cost-report generate --format=html -o cost-report.html

# JSON output for jq or downstream processing
aws-cost-report generate --format=json > monthly-costs.json

# CSV output for spreadsheets
aws-cost-report generate --format=csv > monthly-costs.csv
```

### 16. Webhooks & ChatOps (Slack, Microsoft Teams, Generic HTTP)
Automate spending visibility and budget alerts across your alerting channels:

```bash
# Post rich Block Kit cards to Slack
aws-cost-report generate --slack-webhook="https://hooks.slack.com/services/..."

# Post MessageCard alerts to Microsoft Teams
aws-cost-report generate --teams-webhook="https://outlook.office.com/webhook/..."

# Post structured JSON payload to any generic HTTP/HTTPS webhook
aws-cost-report generate --webhook-url="https://api.example.com/webhooks/finops"
```

### 17. Multi-Currency Support (Auto-Detected & Configurable)
`aws-cost-report` automatically detects your AWS account's billing currency (`EUR`, `GBP`, `JPY`, `AUD`, `CAD`, `USD`, etc.) from the AWS Cost Explorer API response and dynamically renders the corresponding symbols (`€`, `£`, `¥`, `A$`, `CA$`, `$`) and headers (`Diff (€)`, `Delta (£)`).

You can also explicitly set or override the currency:

```bash
# Explicitly set currency for reports
aws-cost-report generate --currency=EUR

# Compare spend in British Pounds
aws-cost-report generate --currency=GBP --compare
```

### 18. Shell Autocompletion (`completion`)
Generate command and flag autocompletion scripts for your shell:

```bash
# Bash
source <(aws-cost-report completion bash)

# Zsh
source <(aws-cost-report completion zsh)

# Fish
aws-cost-report completion fish | source
```

### 19. Save Reports Directly to Amazon S3
Export reports and JSON summaries straight to Amazon S3 buckets using `s3://` URIs or the dedicated `--s3-bucket` and `--s3-key` flags:

```bash
# Export markdown report directly to an S3 URI:
aws-cost-report generate --compare --output="s3://my-billing-reports/2026-10-report.md"

# Export both formatted report and machine-readable JSON summary to S3:
aws-cost-report generate --compare \
  --output="s3://my-billing-reports/cost-report.md" \
  --json-summary="s3://my-billing-reports/cost-summary.json"

# Alternatively, specify target bucket with local output path:
aws-cost-report generate --output="report.csv" --format=csv --s3-bucket="my-billing-reports" --s3-key="csv/2026-10.csv"
```

### 20. Enterprise Discount Program (EDP) & Net Cost Metrics
For AWS accounts with negotiated Enterprise Discount Programs (EDP) or private pricing agreements, `aws-cost-report` supports AWS Cost Explorer net metrics (`NetUnblendedCost` and `NetAmortizedCost`):

```bash
# Query net discount metrics alongside standard costs:
aws-cost-report generate --compare --metrics="UnblendedCost,NetUnblendedCost,NetAmortizedCost"

# Or query all supported metrics at once:
aws-cost-report generate --compare --metrics=all
```

### 21. Charge Type Filtering & Excluding Credits (`--charge-type`, `--exclude-credits`)
Filter AWS line items by charge/record type or exclude promotional credits and refunds from Month-over-Month calculations:

```bash
# Exclude AWS promotional credits and refunds:
aws-cost-report generate --compare --exclude-credits

# Isolate operational usage and tax:
aws-cost-report generate --compare --charge-type="Usage,Tax"

# Audit upfront reservation and savings plan fees:
aws-cost-report generate --compare --charge-type="UpfrontReservationFee"
```

### 22. Regional Cost Breakdown & Filtering (`--group-by=REGION`, `--region`)
Analyze AWS infrastructure spend across geographical regions or isolate specific target regions:

```bash
# Group and compare total spend by AWS Region
aws-cost-report generate --compare --group-by=REGION

# Multi-dimensional grouping: Services within each Region
aws-cost-report generate --compare --group-by=REGION,SERVICE

# Filter cost explorer queries to specific AWS regions
aws-cost-report generate --compare --region="us-east-1,eu-west-1"
```

### 23. Cross-Account Role Assumption via AWS STS (`--assume-role-arn`)
Audit multiple AWS member accounts from a central security/billing account using AWS STS `AssumeRole`:

```bash
# Assume an IAM role in target AWS member account:
aws-cost-report generate --compare \
  --assume-role-arn="arn:aws:iam::123456789012:role/FinOpsCostAuditor" \
  --role-session-name="cost-report-audit"
```

### 24. Cost Allocation Tag Discovery (`tags`)
Inspect active Cost Allocation Tags configured in your AWS account to see what tag keys and values exist:

```bash
# List all active Cost Allocation Tag keys:
aws-cost-report tags

# List all active tag values for a specific tag key:
aws-cost-report tags --tag-key="Environment"

# Export tag keys as machine-readable JSON:
aws-cost-report tags --format=json
```

### 25. AWS Cost Anomaly Detection (`anomalies`)
Identify sudden spend spikes and unexpected cloud billing anomalies detected by AWS machine learning:

```bash
# Query recent cost anomalies for the billing period:
aws-cost-report anomalies

# Export anomaly findings to JSON for automated alerting:
aws-cost-report anomalies --format=json
```

---

## GitHub Action

`aws-cost-report` is available as a first-class GitHub Action that can be dropped into any workflow with zero setup. It automatically downloads pre-built native binaries, runs cost audits, provides Month-over-Month comparisons, outputs structured metrics to workflow steps, and writes beautiful markdown reports into `$GITHUB_STEP_SUMMARY`.

### Quickstart

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

### Action Inputs

| Input | Description | Default | Required |
|:---|:---|:---|:---:|
| `account-name` | Alias for `name` | `""` | No |
| `assume-role-arn` | AWS IAM Role ARN to assume via STS for cross-account cost auditing | `""` | No |
| `charge-type` | Filter costs by charge/record type (e.g. `Usage,Tax`, `Credit`) | `""` | No |
| `compare` | Enable Month-over-Month comparison against preceding period (`true`/`false`) | `true` | No |
| `config` | Path to custom YAML configuration file | `""` | No |
| `currency` | Override currency code and symbol (e.g. `USD`, `EUR`, `GBP`). Auto-detected from AWS by default. | `""` | No |
| `daily` | Query daily spend granularity over the billing period (`true`/`false`) | `false` | No |
| `end` | Explicit billing end date (`YYYY-MM-01` or `YYYY-MM-DD`) | `""` | No |
| `exclude-credits` | Exclude promotional credits and refunds (`true`/`false`) | `false` | No |
| `fail-on-threshold` | Whether to terminate step with failure on threshold breach (`true`/`false`) | `true` | No |
| `filter` | Comma-separated substring filter for services (e.g. `DynamoDB,Lambda`) | `""` | No |
| `forecast` | Query AWS Cost Explorer projected month-end spend forecast and guardrail (`true`/`false`) | `false` | No |
| `format` | Report output format (`markdown`, `table`, `html`, `json`, `csv`) | `markdown` | No |
| `group-by` | Grouping dimension(s), up to 2 comma-separated (`SERVICE`, `LINKED_ACCOUNT`, `TAG:<Key>`) | `SERVICE` | No |
| `min-cost` | Minimum cost floor to display in itemized breakdown | `""` | No |
| `mock` | Run with offline deterministic mock data (no AWS API calls) | `false` | No |
| `monthly` | Render multi-month historical breakdown in column format (`true`/`false`) | `false` | No |
| `months` | Number of historical months back to query | `1` | No |
| `mtd` | Query Month-to-Date spend from 1st of month up to today (`true`/`false`) | `false` | No |
| `name` | Human-readable name or alias for the AWS account (e.g. `Production Core`) | `""` | No |
| `no-cache` | Disable local query response caching (`true`/`false`) | `false` | No |
| `output-file` | Optional path to save the formatted report to disk or `s3://` URI | `""` | No |
| `record-type` | Alias for `charge-type` | `""` | No |
| `region` | Filter costs by comma-separated AWS regions (e.g. `us-east-1,eu-west-1`) | `""` | No |
| `role-session-name` | Optional session name when assuming an IAM role | `""` | No |
| `role-to-assume` | Alias for `assume-role-arn` | `""` | No |
| `s3-bucket` | Target S3 bucket name for saving report output | `""` | No |
| `s3-key` | Optional S3 object key prefix/path (default: based on output file name) | `""` | No |
| `slack-webhook` | Incoming Slack webhook URL for automated ChatOps notifications | `""` | No |
| `start` | Explicit billing start date (`YYYY-MM-01` or `YYYY-MM-DD`) | `""` | No |
| `tag` | Filter costs by AWS Cost Allocation Tag (e.g. `Environment=Production` or `Environment=Prod,Stage`) | `""` | No |
| `tag-filter` | Alias for `tag` | `""` | No |
| `teams-webhook` | Incoming Microsoft Teams webhook URL for automated ChatOps notifications | `""` | No |
| `threshold` | SRE budget alert dollar threshold (e.g. `500.00`). Exceeding fails the step. | `""` | No |
| `top` | Limit breakdown to top N cost drivers and aggregate remainder into "Other" | `""` | No |
| `version` | Version of `aws-cost-report` binary to run (e.g. `v0.7.0` or `latest`) | `latest` | No |
| `webhook-url` | Generic HTTP/HTTPS webhook URL for automated JSON cost report dispatch | `""` | No |
| `write-summary` | Automatically publish markdown table to `$GITHUB_STEP_SUMMARY` | `true` | No |

### Action Outputs

| Output | Description | Example |
|:---|:---|:---|
| `cost-delta` | Absolute dollar difference (`total - previous`) | `0.45` |
| `cost-delta-pct` | Percentage change in total cost | `2.13` |
| `forecast-amount` | Projected month-end spend amount if `--forecast` enabled | `28.50` |
| `forecast-breached` | `true` if projected month-end spend breached `threshold` | `false` |
| `previous-cost` | Preceding period total unblended cost | `21.13` |
| `report` | Full formatted cost report string | Multi-line table/markdown |
| `report-file` | Destination file path if `output-file` was set | `./cost-report.md` |
| `threshold-breached` | `true` if total cost exceeded configured `threshold` | `false` |
| `total-cost` | Current period total unblended cost | `21.58` |

### Automated PR / Slack Notification Example

```yaml
      - name: Run AWS Cost Report
        id: cost
        uses: smford/aws-cost-report@latest # or @v0.7.0
        with:
          compare: 'true'
          threshold: '1000.00'
          fail-on-threshold: 'false'

      - name: Post Cost Summary on PR
        if: github.event_name == 'pull_request'
        uses: actions/github-script@60a0d83039c74a4aee543508d2ffcb1c3799cdea # v7.0.1
        with:
          script: |
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: `### 💸 AWS Cost Report\n**Total:** $${{ steps.cost.outputs.total-cost }} (Delta: ${{ steps.cost.outputs.cost-delta-pct }}%)\n\nThreshold Breached: ${{ steps.cost.outputs.threshold-breached }}`
            })
```

---

## GitHub Actions CI/CD Integration (Manual CLI Script)

```yaml
name: Monthly AWS Cost Report

on:
  schedule:
    - cron: '0 9 1 * *' # Runs 1st of every month at 09:00 UTC
  workflow_dispatch:

permissions:
  id-token: write
  contents: read

jobs:
  cost-audit:
    runs-on: ubuntu-latest
    steps:
      - name: Configure AWS Credentials via OIDC
        uses: aws-actions/configure-aws-credentials@ececac1a45f3b08a01d2dd070d28d111c5fe6722 # v4.1.0
        with:
          role-to-assume: arn:aws:iam::123456789012:role/GitHubActionsCostReport
          aws-region: us-east-1

      - name: Install aws-cost-report
        run: |
          curl -sSL https://github.com/smford/aws-cost-report/releases/download/v0.7.0/aws-cost-report_0.7.0_linux_amd64.tar.gz | tar -xz
          sudo mv aws-cost-report /usr/local/bin/

      - name: Run Cost Audit with Budget Guardrail
        run: |
          aws-cost-report generate \
            --compare \
            --format=markdown \
            --fail-on-threshold=500.00
```

---

## Configuration File (`config.yaml`)

All CLI options can be configured via a `config.yaml` file placed in the working directory or specified with `--config`:

```yaml
version: "1"
name: "Production Core" # Friendly alias displayed alongside AWS Account ID in report headers
defaults:
  currency: "USD"
  months_back: 1
  min_display_cost: 0.01
  top: 10 # 0 displays all, >0 displays top N and aggregates remainder into "Other"
  metrics:
    - "UnblendedCost"
    - "BlendedCost"
    - "AmortizedCost"

grouping:
  type: "DIMENSION" # DIMENSION or TAG
  key: "SERVICE"    # SERVICE, LINKED_ACCOUNT, or Tag Key

budget:
  fail_on_threshold: 0.00 # Set > 0 to enable exit-1 threshold guardrail

reporting:
  format: "table" # table, markdown, json, csv
  write_job_summary: true

# Friendly account aliases for LINKED_ACCOUNT grouping
accounts:
  "123456789012": "Production Core"
  "987654321098": "Staging / QA"

# Optional incoming ChatOps webhooks (supports environment variable expansion)
slack_webhook: "${SLACK_WEBHOOK_URL:-}"
teams_webhook: "${TEAMS_WEBHOOK_URL:-}"
```

*Note: Environment variables in `${VAR}` or `${VAR:-default}` syntax are automatically expanded when loading `config.yaml`. Command-line flags always take highest precedence.*

---

## HTTP Daemon Mode (`--server`)

`aws-cost-report` can run as an HTTP microservice for health monitoring, Prometheus/blackbox probes, or dynamic cost dashboards:

```bash
aws-cost-report --server --mock
```

- **`GET /healthz`**: Liveness probe returning `{"status":"ok","version":"..."}`.
- **`GET /readyz`**: Readiness probe verifying AWS credential authentication and STS caller identity (or mock readiness).
- **`GET /metrics`**: Prometheus metrics exposition endpoint (`text/plain; version=0.0.4`) exposing account totals, service-by-service costs, MoM deltas, and budget guardrail alerts for Prometheus and Grafana.
- **`GET /report`**: Generates reports dynamically with 15-minute in-memory caching to avoid AWS Cost Explorer rate limits (`LimitExceededException`).
  - Supports query parameters: `?format=markdown&compare=true&top=5&months=1&group-by=SERVICE`. Format options: `table`, `markdown`, `json`, `csv`, `prometheus`.

---

## Required AWS IAM Permissions

To run `aws-cost-report`, the executing IAM principal requires read permissions for Cost Explorer and STS:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CostExplorerReadOnly",
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetDimensionValues",
        "ce:GetTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "STSCallerIdentity",
      "Effect": "Allow",
      "Action": "sts:GetCallerIdentity",
      "Resource": "*"
    },
    {
      "Sid": "OptionalAccountAliasLookup",
      "Effect": "Allow",
      "Action": "iam:ListAccountAliases",
      "Resource": "*"
    }
  ]
}

*Note: `iam:ListAccountAliases` is optional; it enables automatic discovery of your IAM account alias when `--name` is omitted.*
```

---

## CLI Reference

### Commands

| Command | Description |
|:---|:---|
| `generate` | Generate monthly billing and usage cost report (default report engine) |
| `services` | List official AWS Cost Explorer service names available in your account |
| `tags` | List active AWS Cost Allocation Tag keys or tag values |
| `anomalies` | List recent AWS Cost Anomalies detected by Cost Explorer |
| `server` | Run as an HTTP service daemon exposing `/healthz`, `/readyz`, `/report`, and `/metrics` |
| `completion` | Generate shell autocompletion script (`bash`, `zsh`, `fish`) |
| `version` | Display application version, commit SHA, build date, and license |
| `help` | Display command-line usage and help information |

*Note: Running `aws-cost-report` without any arguments immediately displays the help screen with zero pause or network latency.*

### Command Options & Flags

| Flag | Short | Default | Description |
|:---|:---|:---|:---|
| `--compare` | | `false` | Enable Month-over-Month (MoM) comparison against preceding billing period |
| `--mtd` | `--current-month` | `false` | Query Month-to-Date spend from the 1st of the current month up to today |
| `--daily` | | `false` | Query daily spend granularity over the billing period |
| `--granularity` | | `MONTHLY` | Spend granularity: `MONTHLY` (default) or `DAILY` |
| `--forecast` | | `false` | Query AWS Cost Explorer machine learning forecast for projected month-end spend |
| `--monthly` | `--trend` | `false` | Display multi-month historical breakdown with each month as a separate column (default: 12 months) |
| `--format` | | `table` | Output format: `table`, `markdown`, `html`, `json`, `csv`, `prometheus` |
| `--filter` | `--service` | | Filter services by official name (`Amazon DynamoDB`), alias (`S3`, `EC2`), or substring |
| `--tag` | `--tag-filter` | | Filter costs by AWS Cost Allocation Tag (e.g. `Environment=Production` or `Team=Core,API`) |
| `--top` | | `0` | Show top N highest cost items and aggregate remainder into "Other Services" |
| `--months` | | `1` | Number of previous complete months to analyze |
| `--start` | | | Explicit billing start date (`YYYY-MM-01` or `YYYY-MM-DD`, inclusive) |
| `--end` | | | Explicit billing end date (`YYYY-MM-01` or `YYYY-MM-DD`, inclusive) |
| `--metrics` | | `UnblendedCost,BlendedCost,AmortizedCost` | Comma-separated cost metrics (`UnblendedCost`, `BlendedCost`, `AmortizedCost`, `NetUnblendedCost`, `NetAmortizedCost`, or `all` for EDP pricing) |
| `--group-by` | | `SERVICE` | Grouping dimension(s), up to 2 comma-separated (`SERVICE`, `LINKED_ACCOUNT`, `REGION`, `TAG:<Key>`) |
| `--region` | `--filter-region` | | Filter costs by comma-separated AWS regions (e.g. `us-east-1,eu-west-1`) |
| `--assume-role-arn` | | | AWS IAM Role ARN to assume via STS for cross-account cost auditing |
| `--role-session-name` | | `aws-cost-report` | Optional session name when assuming an IAM role |
| `--tag-key` | | | Query tag values for a specific tag key when running `tags` command |
| `--name` | `--account-name` | | Human-readable name or alias for the AWS account (e.g. `Production Core`) |
| `--threshold` | | `0.00` | SRE budget alert dollar threshold. Exits with code 1 if exceeded |
| `--min-cost` | | `0.01` | Minimum cost threshold to display line items |
| `--output` | `-o` | | Write formatted cost report to specified local file path or `s3://bucket/key` URI |
| `--json-summary` | | | Write machine-readable JSON metrics summary to file path or `s3://bucket/key` URI |
| `--s3-bucket` | | | Target AWS S3 bucket name for saving report output |
| `--s3-key` | | | Optional S3 object key prefix/path for output upload |
| `--slack-webhook` | | | Incoming Slack webhook URL to post formatted Block Kit report |
| `--teams-webhook` | | | Incoming Microsoft Teams webhook URL to post formatted MessageCard report |
| `--webhook-url` | | | Generic HTTP/HTTPS webhook URL to post formatted cost report JSON payload |
| `--no-color` | | `false` | Disable ANSI color codes in table deltas (also honors `NO_COLOR=1`) |
| `--no-cache` | | `false` | Disable local query response disk caching |
| `--cache-dir` | | | Custom local query response cache directory path (defaults to user cache dir) |
| `--clear-cache` | | | Purge all locally cached query results from disk and exit |
| `--mock` | | `false` | Run in offline mode with simulated AWS billing data |
| `--config` | `-c` | `config.yaml` | Path to YAML configuration file |
| `--version` | `-v` | | Display version metadata and license |

---

## License

`aws-cost-report` is open source software licensed under the [MIT License](LICENSE).
