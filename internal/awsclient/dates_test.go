package awsclient

import (
	"testing"
	"time"
)

func TestResolveTimePeriodDefaults(t *testing.T) {
	// Fixed date: Oct 28, 2026
	refTime := time.Date(2026, time.October, 28, 14, 0, 0, 0, time.UTC)

	// 1 month back -> September 2026
	dr, err := ResolveTimePeriod(refTime, 1, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dr.Start.Format(DateFormat) != "2026-09-01" {
		t.Errorf("expected start 2026-09-01, got %s", dr.Start.Format(DateFormat))
	}
	if dr.EndInclusive.Format(DateFormat) != "2026-09-30" {
		t.Errorf("expected inclusive end 2026-09-30, got %s", dr.EndInclusive.Format(DateFormat))
	}
	if dr.EndExclusiveAWS.Format(DateFormat) != "2026-10-01" {
		t.Errorf("expected AWS exclusive end 2026-10-01, got %s", dr.EndExclusiveAWS.Format(DateFormat))
	}
	if dr.BillingMonthDesc != "September 2026" {
		t.Errorf("expected BillingMonthDesc 'September 2026', got %q", dr.BillingMonthDesc)
	}
	if dr.IsEstimated {
		t.Errorf("expected IsEstimated false for historical month")
	}

	// 3 months back -> July 2026 - September 2026
	dr3, err := ResolveTimePeriod(refTime, 3, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr3.Start.Format(DateFormat) != "2026-07-01" {
		t.Errorf("expected start 2026-07-01, got %s", dr3.Start.Format(DateFormat))
	}
	if dr3.EndInclusive.Format(DateFormat) != "2026-09-30" {
		t.Errorf("expected inclusive end 2026-09-30, got %s", dr3.EndInclusive.Format(DateFormat))
	}
	if dr3.EndExclusiveAWS.Format(DateFormat) != "2026-10-01" {
		t.Errorf("expected AWS exclusive end 2026-10-01, got %s", dr3.EndExclusiveAWS.Format(DateFormat))
	}
}

func TestResolveTimePeriodExplicitDates(t *testing.T) {
	refTime := time.Date(2026, time.October, 28, 14, 0, 0, 0, time.UTC)

	// User specifies 2026-09-01 to 2026-09-30
	dr, err := ResolveTimePeriod(refTime, 0, "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr.Start.Format(DateFormat) != "2026-09-01" {
		t.Errorf("expected start 2026-09-01, got %s", dr.Start.Format(DateFormat))
	}
	if dr.EndInclusive.Format(DateFormat) != "2026-09-30" {
		t.Errorf("expected endInclusive 2026-09-30, got %s", dr.EndInclusive.Format(DateFormat))
	}
	if dr.EndExclusiveAWS.Format(DateFormat) != "2026-10-01" {
		t.Errorf("expected endExclusiveAWS 2026-10-01, got %s", dr.EndExclusiveAWS.Format(DateFormat))
	}

	// User specifies first day of next month as end: 2026-09-01 to 2026-10-01
	dr2, err := ResolveTimePeriod(refTime, 0, "2026-09-01", "2026-10-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr2.EndInclusive.Format(DateFormat) != "2026-09-30" {
		t.Errorf("expected endInclusive 2026-09-30, got %s", dr2.EndInclusive.Format(DateFormat))
	}
	if dr2.EndExclusiveAWS.Format(DateFormat) != "2026-10-01" {
		t.Errorf("expected endExclusiveAWS 2026-10-01, got %s", dr2.EndExclusiveAWS.Format(DateFormat))
	}

	// Invalid date ordering
	_, err = ResolveTimePeriod(refTime, 0, "2026-10-01", "2026-09-01")
	if err == nil {
		t.Errorf("expected error for inverted dates")
	}
}

func TestResolvePreviousPeriod(t *testing.T) {
	refTime := time.Date(2026, time.October, 28, 14, 0, 0, 0, time.UTC)
	current, err := ResolveTimePeriod(refTime, 1, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prev := ResolvePreviousPeriod(current)
	if prev.Start.Format(DateFormat) != "2026-08-01" {
		t.Errorf("expected prev start 2026-08-01, got %s", prev.Start.Format(DateFormat))
	}
	if prev.EndInclusive.Format(DateFormat) != "2026-08-31" {
		t.Errorf("expected prev inclusive end 2026-08-31, got %s", prev.EndInclusive.Format(DateFormat))
	}
	if prev.EndExclusiveAWS.Format(DateFormat) != "2026-09-01" {
		t.Errorf("expected prev exclusive AWS end 2026-09-01, got %s", prev.EndExclusiveAWS.Format(DateFormat))
	}
	if prev.BillingMonthDesc != "August 2026" {
		t.Errorf("expected prev description August 2026, got %s", prev.BillingMonthDesc)
	}
}

func TestResolvePreviousPeriodCustomRange(t *testing.T) {
	refTime := time.Date(2026, time.October, 28, 14, 0, 0, 0, time.UTC)
	// 10-day custom span: 2026-10-11 to 2026-10-20 (inclusive)
	current, err := ResolveTimePeriod(refTime, 0, "2026-10-11", "2026-10-20")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prev := ResolvePreviousPeriod(current)
	// Prior 10 days should end on 2026-10-10 and start on 2026-10-01
	if prev.EndInclusive.Format(DateFormat) != "2026-10-10" {
		t.Errorf("expected prev inclusive end 2026-10-10, got %s", prev.EndInclusive.Format(DateFormat))
	}
	if prev.Start.Format(DateFormat) != "2026-10-01" {
		t.Errorf("expected prev start 2026-10-01, got %s", prev.Start.Format(DateFormat))
	}
}

func TestResolveTimePeriodMTD(t *testing.T) {
	refTime := time.Date(2026, time.October, 29, 14, 0, 0, 0, time.UTC)
	dr, err := ResolveTimePeriod(refTime, 1, "", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dr.Start.Format(DateFormat) != "2026-10-01" {
		t.Errorf("expected start 2026-10-01, got %s", dr.Start.Format(DateFormat))
	}
	if dr.EndInclusive.Format(DateFormat) != "2026-10-29" {
		t.Errorf("expected end inclusive 2026-10-29, got %s", dr.EndInclusive.Format(DateFormat))
	}
	if dr.EndExclusiveAWS.Format(DateFormat) != "2026-10-30" {
		t.Errorf("expected AWS exclusive end 2026-10-30, got %s", dr.EndExclusiveAWS.Format(DateFormat))
	}
	if dr.BillingMonthDesc != "October 2026 (Month-to-Date)" {
		t.Errorf("expected billing month desc 'October 2026 (Month-to-Date)', got %q", dr.BillingMonthDesc)
	}
	if !dr.IsEstimated {
		t.Errorf("expected IsEstimated true for MTD")
	}
	if !dr.IsMTD {
		t.Errorf("expected IsMTD true")
	}

	// Test ResolvePreviousPeriod for MTD
	prev := ResolvePreviousPeriod(dr)
	if prev.Start.Format(DateFormat) != "2026-09-01" {
		t.Errorf("expected prev start 2026-09-01, got %s", prev.Start.Format(DateFormat))
	}
	if prev.EndInclusive.Format(DateFormat) != "2026-09-29" {
		t.Errorf("expected prev end inclusive 2026-09-29, got %s", prev.EndInclusive.Format(DateFormat))
	}
	if prev.EndExclusiveAWS.Format(DateFormat) != "2026-09-30" {
		t.Errorf("expected prev AWS exclusive end 2026-09-30, got %s", prev.EndExclusiveAWS.Format(DateFormat))
	}
	if prev.BillingMonthDesc != "September 2026 (Month-to-Date)" {
		t.Errorf("expected prev billing month desc 'September 2026 (Month-to-Date)', got %q", prev.BillingMonthDesc)
	}
}
