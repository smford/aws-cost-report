package renderer

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/smford/aws-cost-report/internal/awsclient"
)

// RenderHTML generates a standalone, single-file interactive HTML dashboard report.
func RenderHTML(w io.Writer, r *awsclient.CostReport, threshold float64) error {
	var b strings.Builder

	sym := r.Metadata.CurrencySymbol()
	cur := r.Metadata.Currency

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AWS Cost Report — ` + html.EscapeString(r.Metadata.BillingMonth) + `</title>
  <style>
    :root {
      --bg: #0d1117;
      --card: #161b22;
      --border: #30363d;
      --text: #c9d1d9;
      --text-muted: #8b949e;
      --primary: #238636;
      --accent: #58a6ff;
      --danger: #f85149;
      --danger-bg: rgba(248, 81, 73, 0.15);
      --success: #3fb950;
      --warning: #d29922;
      --bar-bg: #21262d;
    }
    @media (prefers-color-scheme: light) {
      :root {
        --bg: #f6f8fa;
        --card: #ffffff;
        --border: #d0d7de;
        --text: #1f2328;
        --text-muted: #656d76;
        --primary: #1f883d;
        --accent: #0969da;
        --danger: #cf222e;
        --danger-bg: #ffebe9;
        --success: #1a7f37;
        --warning: #9a6700;
        --bar-bg: #eaeef2;
      }
    }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background: var(--bg);
      color: var(--text);
      margin: 0;
      padding: 32px 16px;
      line-height: 1.5;
    }
    .container {
      max-width: 1080px;
      margin: 0 auto;
    }
    header {
      margin-bottom: 24px;
      border-bottom: 1px solid var(--border);
      padding-bottom: 16px;
    }
    h1 { margin: 0 0 8px; font-size: 1.75rem; display: flex; align-items: center; gap: 8px; }
    .subtitle { color: var(--text-muted); font-size: 0.95rem; margin: 0; }
    .alert-banner {
      background: var(--danger-bg);
      border: 1px solid var(--danger);
      color: var(--danger);
      padding: 12px 16px;
      border-radius: 6px;
      margin-bottom: 24px;
      font-weight: 600;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 16px;
      margin-bottom: 24px;
    }
    .card {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 16px;
    }
    .card-title { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-muted); margin-bottom: 4px; }
    .card-value { font-size: 1.4rem; font-weight: 700; }
    .card-sub { font-size: 0.85rem; color: var(--text-muted); margin-top: 4px; }
    .delta-pos { color: var(--danger); font-weight: 600; }
    .delta-neg { color: var(--success); font-weight: 600; }
    .delta-zero { color: var(--text-muted); }
    table {
      width: 100%;
      border-collapse: collapse;
      margin-bottom: 24px;
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 6px;
      overflow: hidden;
    }
    th, td {
      padding: 10px 14px;
      text-align: left;
      border-bottom: 1px solid var(--border);
      font-size: 0.9rem;
    }
    th {
      background: var(--bar-bg);
      font-weight: 600;
      color: var(--text);
    }
    tr:last-child td { border-bottom: none; }
    tr:hover { background: rgba(127, 127, 127, 0.05); }
    .text-right { text-align: right; }
    .text-center { text-align: center; }
    .font-mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    .badge {
      display: inline-block;
      padding: 2px 8px;
      font-size: 0.75rem;
      font-weight: 600;
      border-radius: 12px;
      background: var(--bar-bg);
      border: 1px solid var(--border);
    }
    .progress-bar-container {
      width: 100%;
      background: var(--bar-bg);
      border-radius: 4px;
      height: 8px;
      overflow: hidden;
    }
    .progress-bar {
      height: 100%;
      background: var(--accent);
      border-radius: 4px;
    }
    footer {
      text-align: center;
      color: var(--text-muted);
      font-size: 0.8rem;
      margin-top: 40px;
      padding-top: 16px;
      border-top: 1px solid var(--border);
    }
    footer a { color: var(--accent); text-decoration: none; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>📊 AWS Monthly Billing & Usage Report</h1>
      <p class="subtitle">`)

	accountTitle := r.Metadata.AccountID
	if r.Metadata.AccountName != "" {
		accountTitle = fmt.Sprintf("%s (%s)", r.Metadata.AccountName, r.Metadata.AccountID)
	}
	b.WriteString(fmt.Sprintf("<strong>Account:</strong> %s &bull; <strong>Billing Month:</strong> %s &bull; <strong>Period:</strong> %s to %s",
		html.EscapeString(accountTitle),
		html.EscapeString(r.Metadata.BillingMonth),
		r.Metadata.PeriodStart,
		r.Metadata.PeriodEnd))
	b.WriteString(`</p>
    </header>
`)

	// Budget Breach Alert Banner
	thresholdBreached := threshold > 0 && r.Totals.UnblendedCost > threshold
	forecastBreached := r.Forecast != nil && threshold > 0 && r.Forecast.TotalAmount > threshold
	if thresholdBreached {
		b.WriteString(fmt.Sprintf(`    <div class="alert-banner">
      ⚠️ <span><strong>BUDGET ALERT:</strong> Total spend (%s%.2f %s) exceeds configured guardrail threshold of %s%.2f %s!</span>
    </div>
`, sym, r.Totals.UnblendedCost, cur, sym, threshold, cur))
	} else if forecastBreached {
		b.WriteString(fmt.Sprintf(`    <div class="alert-banner">
      ⚠️ <span><strong>PREDICTIVE BUDGET ALERT:</strong> Projected month-end spend (%s%.2f %s) is on track to breach configured threshold of %s%.2f %s!</span>
    </div>
`, sym, r.Forecast.TotalAmount, cur, sym, threshold, cur))
	}

	// KPI Cards
	b.WriteString(`    <div class="grid">
      <div class="card">
        <div class="card-title">Total Spend (` + cur + `)</div>
        <div class="card-value font-mono">` + fmt.Sprintf("%s%.2f", sym, r.Totals.UnblendedCost) + `</div>
        <div class="card-sub">`)
	if r.HasComparison {
		dStr, dClass := formatHTMLDelta(r.Totals.MoMDeltaAmount, r.Totals.MoMDeltaPercent, sym)
		b.WriteString(fmt.Sprintf(`<span class="%s">%s vs prev month</span>`, dClass, dStr))
	} else {
		b.WriteString(html.EscapeString(r.Metadata.EstimatedOrFinal))
	}
	b.WriteString(`</div>
      </div>
      <div class="card">
        <div class="card-title">Previous Month</div>
        <div class="card-value font-mono">` + fmt.Sprintf("%s%.2f", sym, r.Totals.PrevUnblendedCost) + `</div>
        <div class="card-sub">Preceding billing cycle</div>
      </div>
`)

	if r.Forecast != nil {
		b.WriteString(`      <div class="card">
        <div class="card-title">Projected Month-End</div>
        <div class="card-value font-mono">` + fmt.Sprintf("%s%.2f", sym, r.Forecast.TotalAmount) + `</div>
        <div class="card-sub">Est. ` + fmt.Sprintf("%s%.2f", sym, r.Forecast.MeanAmount) + ` remaining</div>
      </div>
`)
	}

	b.WriteString(`      <div class="card">
        <div class="card-title">Active Line Items</div>
        <div class="card-value">` + fmt.Sprintf("%d", r.Metadata.ActiveServices) + `</div>
        <div class="card-sub">Grouped by ` + html.EscapeString(r.Metadata.GroupKey) + `</div>
      </div>
    </div>
`)

	// Metadata Table
	b.WriteString(`    <h3>📋 Configuration & Filters</h3>
    <table>
      <tbody>
        <tr><th>Account</th><td>` + html.EscapeString(accountTitle) + `</td><th>Status</th><td><span class="badge">` + html.EscapeString(r.Metadata.EstimatedOrFinal) + `</span></td></tr>
        <tr><th>Currency</th><td>` + html.EscapeString(r.Metadata.Currency) + ` (` + sym + `)</td><th>Grouping</th><td>` + html.EscapeString(r.Metadata.GroupKey) + `</td></tr>
`)
	if r.Metadata.TagFilter != "" {
		b.WriteString(`        <tr><th>Tag Filter</th><td colspan="3"><span class="badge">` + html.EscapeString(r.Metadata.TagFilter) + `</span></td></tr>
`)
	}
	if r.Metadata.ChargeTypeFilter != "" {
		b.WriteString(`        <tr><th>Charge Types</th><td colspan="3"><span class="badge">` + html.EscapeString(r.Metadata.ChargeTypeFilter) + `</span></td></tr>
`)
	} else if r.Metadata.ExcludeCredits {
		b.WriteString(`        <tr><th>Credits & Refunds</th><td colspan="3"><span class="badge">Excluded</span></td></tr>
`)
	}
	b.WriteString(`      </tbody>
    </table>
`)

	// Itemized Table
	groupLabel := r.Metadata.GroupKey
	if groupLabel == "" || strings.EqualFold(groupLabel, "SERVICE") {
		groupLabel = "Service Name"
	}

	b.WriteString(`    <h3>🔍 Cost Breakdown by ` + html.EscapeString(groupLabel) + `</h3>
    <table>
      <thead>
        <tr>
          <th>` + html.EscapeString(groupLabel) + `</th>
          <th class="text-right">Unblended (` + cur + `)</th>
`)
	if r.HasComparison {
		b.WriteString(`          <th class="text-right">Previous (` + cur + `)</th>
          <th class="text-right">Delta (` + sym + `)</th>
          <th class="text-right">Change (%)</th>
`)
	}
	b.WriteString(`          <th class="text-right">Share</th>
        </tr>
      </thead>
      <tbody>
`)

	totalSpend := r.Totals.UnblendedCost
	if totalSpend <= 0 {
		totalSpend = 1.0
	}

	for _, item := range r.Items {
		pctOfTotal := (item.UnblendedCost / totalSpend) * 100.0
		if pctOfTotal < 0 {
			pctOfTotal = 0
		}
		if pctOfTotal > 100 {
			pctOfTotal = 100
		}

		b.WriteString(fmt.Sprintf(`        <tr>
          <td><strong>%s</strong></td>
          <td class="text-right font-mono">%s%.2f</td>
`, html.EscapeString(item.Name), sym, item.UnblendedCost))

		if r.HasComparison {
			dStr, dClass := formatHTMLDelta(item.MoMDeltaAmount, item.MoMDeltaPercent, sym)
			pctStr := fmt.Sprintf("%.2f%%", item.MoMDeltaPercent)
			if item.MoMDeltaPercent > 0 {
				pctStr = "+" + pctStr
			}
			b.WriteString(fmt.Sprintf(`          <td class="text-right font-mono">%s%.2f</td>
          <td class="text-right font-mono %s">%s</td>
          <td class="text-right font-mono %s">%s</td>
`, sym, item.PrevUnblendedCost, dClass, dStr, dClass, pctStr))
		}

		b.WriteString(fmt.Sprintf(`          <td style="width: 120px;" class="text-right">
            <span class="font-mono" style="font-size: 0.8rem;">%.1f%%</span>
            <div class="progress-bar-container"><div class="progress-bar" style="width: %.1f%%;"></div></div>
          </td>
        </tr>
`, pctOfTotal, pctOfTotal))
	}

	b.WriteString(`      </tbody>
    </table>
`)

	// Daily Breakdown Table if present
	if r.DailyBreakdown != nil && len(r.DailyBreakdown.Days) > 0 {
		b.WriteString(`    <h3>📅 Daily Spend Breakdown</h3>
    <table>
      <thead>
        <tr>
          <th>Date</th>
          <th>Day</th>
          <th class="text-right">Spend (` + cur + `)</th>
          <th>Top Cost Driver</th>
        </tr>
      </thead>
      <tbody>
`)
		for _, day := range r.DailyBreakdown.Days {
			b.WriteString(fmt.Sprintf(`        <tr>
          <td class="font-mono">%s</td>
          <td>%s</td>
          <td class="text-right font-mono">%s%.2f</td>
          <td>%s <span class="font-mono" style="color:var(--text-muted);">(%s%.2f)</span></td>
        </tr>
`, day.Date, day.DayOfWeek, sym, day.Total, html.EscapeString(day.TopDriverName), sym, day.TopDriverAmount))
		}
		b.WriteString(fmt.Sprintf(`        <tr style="font-weight: 700; background: var(--bar-bg);">
          <td colspan="2">Total (%d days)</td>
          <td class="text-right font-mono">%s%.2f</td>
          <td>Daily Average: %s%.2f / day</td>
        </tr>
      </tbody>
    </table>
`, len(r.DailyBreakdown.Days), sym, r.DailyBreakdown.GrandTotal, sym, r.DailyBreakdown.Average))
	}

	// Monthly Breakdown Table if present
	if r.MonthlyBreakdown != nil && len(r.MonthlyBreakdown.Months) > 1 {
		b.WriteString(`    <h3>📈 Historical Multi-Month Trend</h3>
    <div style="overflow-x: auto;">
    <table>
      <thead>
        <tr>
          <th>` + html.EscapeString(groupLabel) + `</th>
`)
		for _, m := range r.MonthlyBreakdown.Months {
			b.WriteString(fmt.Sprintf(`          <th class="text-right">%s</th>
`, html.EscapeString(m.Label)))
		}
		b.WriteString(`          <th class="text-right">Total</th>
        </tr>
      </thead>
      <tbody>
`)
		for _, it := range r.MonthlyBreakdown.Items {
			b.WriteString(fmt.Sprintf(`        <tr>
          <td><strong>%s</strong></td>
`, html.EscapeString(it.Name)))
			for _, m := range r.MonthlyBreakdown.Months {
				b.WriteString(fmt.Sprintf(`          <td class="text-right font-mono">%s%.2f</td>
`, sym, it.Costs[m.Key]))
			}
			b.WriteString(fmt.Sprintf(`          <td class="text-right font-mono"><strong>%s%.2f</strong></td>
        </tr>
`, sym, it.Total))
		}
		b.WriteString(`        <tr style="font-weight: 700; background: var(--bar-bg);">
          <td>Total Spend</td>
`)
		for _, m := range r.MonthlyBreakdown.Months {
			b.WriteString(fmt.Sprintf(`          <td class="text-right font-mono">%s%.2f</td>
`, sym, m.Total))
		}
		b.WriteString(fmt.Sprintf(`          <td class="text-right font-mono">%s%.2f</td>
        </tr>
      </tbody>
    </table>
    </div>
`, sym, r.MonthlyBreakdown.GrandTotal))
	}

	b.WriteString(`    <footer>
      <p>Generated automatically by <a href="https://github.com/smford/aws-cost-report" target="_blank">aws-cost-report</a> &bull; Production AWS FinOps & Cost Reporting</p>
    </footer>
  </div>
</body>
</html>
`)

	_, err := io.WriteString(w, b.String())
	return err
}

func formatHTMLDelta(diff, pct float64, sym string) (string, string) {
	if diff > 0 {
		return fmt.Sprintf("+%s%.2f (+%.2f%%)", sym, diff, pct), "delta-pos"
	} else if diff < 0 {
		return fmt.Sprintf("-%s%.2f (%.2f%%)", sym, -diff, pct), "delta-neg"
	}
	return fmt.Sprintf("%s0.00 (0.00%%)", sym), "delta-zero"
}
