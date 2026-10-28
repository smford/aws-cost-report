# aws-cost-report

[![CI](https://github.com/smford/aws-cost-report/actions/workflows/ci.yml/badge.svg)](https://github.com/smford/aws-cost-report/actions/workflows/ci.yml)
[![SAST](https://github.com/smford/aws-cost-report/actions/workflows/sast.yml/badge.svg)](https://github.com/smford/aws-cost-report/actions/workflows/sast.yml)
[![DAST](https://github.com/smford/aws-cost-report/actions/workflows/dast.yml/badge.svg)](https://github.com/smford/aws-cost-report/actions/workflows/dast.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/smford/aws-cost-report/badge)](https://scorecard.dev/viewer/?uri=github.com/smford/aws-cost-report)
[![GitHub Release](https://img.shields.io/github/v/release/smford/aws-cost-report?logo=github)](https://github.com/smford/aws-cost-report/releases)
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
- **Multi-Dimension Grouping**: Group by AWS `SERVICE`, `LINKED_ACCOUNT`, or custom Cost Allocation Tags (`TAG:Environment`, `TAG:Project`, etc.).
- **SRE Budget Guardrails (`--fail-on-threshold`)**: Blocks CI/CD pipelines by terminating with exit code `1` if total billing exceeds a configured dollar threshold.
- **Multi-Format Output Engines**:
  - `table`: Clean fixed-width ASCII tables formatted like official AWS billing statements.
  - `markdown`: GitHub-flavored Markdown automatically written to `$GITHUB_STEP_SUMMARY` in CI/CD.
  - `json`: Structured JSON array for downstream analytics and log aggregators.
  - `csv`: Standard CSV format with column headers.
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
 Metric             |       Amount (USD) |         Prev (USD) |           Diff ($) |              % Δ
--------------------------------------------------------------------------------------------------------
 Unblended Cost     |              21.58 |              21.13 |               0.45 |            2.13%
 Blended Cost       |              21.59 |              21.14 |               0.45 |            2.13%
 Amortized Cost     |              21.58 |              21.13 |               0.45 |            2.13%
========================================================================================================

--------------------------------------------------------------------------------------------------------
 Service Name                                   |  Unblended (USD) |       Prev (USD) |   Diff ($) |      % Δ
--------------------------------------------------------------------------------------------------------
 Amazon DynamoDB                                |             5.51 |             5.20 |         +0.31 |       +5.96%
 AWS Config                                     |             4.73 |             4.80 |          -0.07 |        -1.46%
 Amazon Cognito                                 |             4.17 |             4.15 |         +0.02 |       +0.48%
 Tax                                            |             2.89 |             2.80 |         +0.09 |       +3.21%
 AWS Key Management Service                     |             1.00 |             1.00 |         +0.00 |       +0.00%
 Amazon Simple Storage Service                  |             0.94 |             0.88 |         +0.06 |       +6.82%
 Amazon Route 53                                |             0.80 |             0.80 |         +0.00 |       +0.00%
 Amazon CloudWatch                              |             0.65 |             0.70 |          -0.05 |        -7.14%
 AWS Secrets Manager                            |             0.40 |             0.40 |         +0.00 |       +0.00%
 AWS Lambda                                     |             0.25 |             0.18 |         +0.07 |      +38.89%
 Amazon Virtual Private Cloud                   |             0.12 |             0.12 |         +0.00 |       +0.00%
 Amazon Simple Notification Service             |             0.06 |             0.05 |         +0.01 |      +20.00%
 AWS Step Functions                             |             0.04 |             0.03 |         +0.01 |      +33.33%
 AWS CloudTrail                                 |             0.02 |             0.02 |         +0.00 |       +0.00%
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
docker pull ghcr.io/smford/aws-cost-report:v0.1.0
docker run --rm \
  -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_REGION=us-east-1 \
  ghcr.io/smford/aws-cost-report:v0.1.0
```

### Go Install

```bash
go install github.com/smford/aws-cost-report/cmd/aws-cost-report@latest
```

---

## Quickstart & Worked Examples

### 1. Basic Report (Previous Complete Month)
By default, queries the last completed calendar month across all active services:

```bash
aws-cost-report
```

### 2. Month-over-Month Comparison
Calculates absolute delta and percentage shift compared to the preceding period:

```bash
aws-cost-report --compare
```

### 3. Service Filtering
Focus on specific workloads or services:

```bash
aws-cost-report --filter="DynamoDB,Lambda,S3"
```

### 4. Group by Tag or Linked Account
Break down costs by environment, team, or AWS Organization linked account:

```bash
# Group by Cost Allocation Tag
aws-cost-report --group-by="TAG:Environment"

# Group by Linked Account
aws-cost-report --group-by="LINKED_ACCOUNT"
```

### 5. Explicit Date Range
Query custom date intervals:

```bash
aws-cost-report --start=2026-08-01 --end=2026-09-30
```

### 6. SRE Budget Guardrail (CI/CD Quality Gate)
Exit with status code `1` if total unblended cost exceeds threshold:

```bash
aws-cost-report --fail-on-threshold=50.00
echo $? # Returns 1 if total cost > $50.00, or 0 if within budget
```

### 7. Export to JSON or CSV

```bash
# JSON output for jq or downstream processing
aws-cost-report --format=json > monthly-costs.json

# CSV output for spreadsheets
aws-cost-report --format=csv > monthly-costs.csv
```

---

## GitHub Actions CI/CD Integration

Deploy `aws-cost-report` into your GitHub Actions workflows using keyless AWS OIDC authentication:

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
          curl -sSL https://github.com/smford/aws-cost-report/releases/download/v0.1.0/aws-cost-report_0.1.0_linux_amd64.tar.gz | tar -xz
          sudo mv aws-cost-report /usr/local/bin/

      - name: Run Cost Audit with Budget Guardrail
        run: |
          aws-cost-report \
            --compare \
            --format=markdown \
            --fail-on-threshold=500.00
```

---

## Configuration File (`config.yaml`)

All CLI options can be configured via a `config.yaml` file placed in the working directory or specified with `--config`:

```yaml
version: "1"
defaults:
  currency: "USD"
  months_back: 1
  min_display_cost: 0.01
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
```

*Note: Command-line flags always take precedence over values in `config.yaml`.*

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
    }
  ]
}
```

---

## CLI Reference

| Flag | Short | Default | Description |
|:---|:---|:---|:---|
| `--config` | `-c` | `config.yaml` | Path to YAML configuration file |
| `--months` | | `1` | Number of previous complete months to analyze |
| `--start` | | | Explicit start date (`YYYY-MM-01`) |
| `--end` | | | Explicit end date (`YYYY-MM-01` or `YYYY-MM-DD`) |
| `--compare` | | `false` | Enable Month-over-Month (MoM) comparison against previous period |
| `--group-by` | | `SERVICE` | Grouping dimension (`SERVICE`, `LINKED_ACCOUNT`) or `TAG:<Key>` |
| `--filter` | `--service` | | Comma-separated list of service name substrings to filter |
| `--metrics` | | `UnblendedCost,BlendedCost,AmortizedCost` | Cost metrics to query or `all` |
| `--min-cost` | | `0.01` | Minimum cost threshold to display line items |
| `--fail-on-threshold` | | `0.00` | Terminate with exit code 1 if total cost exceeds amount |
| `--format` | | `table` | Output format: `table`, `markdown`, `json`, `csv` |
| `--no-color` | | `false` | Disable ANSI color codes in table deltas (also honors `NO_COLOR=1`) |
| `--mock` | | `false` | Run in offline mode with simulated AWS billing data |
| `--version` | `-v` | | Display version metadata and license |

---

## License

`aws-cost-report` is open source software licensed under the [MIT License](LICENSE).
