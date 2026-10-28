package awsclient

import (
	"fmt"
	"time"
)

const DateFormat = "2006-01-02"

// DateRange holds inclusive start, inclusive end, and AWS exclusive end dates.
type DateRange struct {
	Start            time.Time
	EndInclusive     time.Time
	EndExclusiveAWS  time.Time
	BillingMonthDesc string
	IsEstimated      bool
}

// ResolveTimePeriod calculates the query date boundaries adhering to AWS Cost Explorer rules:
// Cost Explorer TimePeriod is inclusive of Start and exclusive of End.
func ResolveTimePeriod(now time.Time, monthsBack int, startStr, endStr string) (*DateRange, error) {
	now = now.UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	if startStr != "" || endStr != "" {
		if startStr == "" || endStr == "" {
			return nil, fmt.Errorf("both --start and --end must be specified when using explicit dates")
		}

		s, err := time.Parse(DateFormat, startStr)
		if err != nil {
			return nil, fmt.Errorf("invalid start date %q (expected YYYY-MM-DD): %w", startStr, err)
		}
		e, err := time.Parse(DateFormat, endStr)
		if err != nil {
			return nil, fmt.Errorf("invalid end date %q (expected YYYY-MM-DD): %w", endStr, err)
		}

		if !s.Before(e) && !s.Equal(e) {
			return nil, fmt.Errorf("start date (%s) must be before or equal to end date (%s)", startStr, endStr)
		}

		// If user passed first day of next month (e.g. 2026-10-01), AWS exclusive is e, inclusive is e - 1 day
		var endInclusive, endExclusive time.Time
		if e.Day() == 1 && (e.After(s)) {
			endExclusive = e
			endInclusive = e.AddDate(0, 0, -1)
		} else {
			// User passed end of month or specific day (e.g. 2026-09-30)
			endInclusive = e
			endExclusive = e.AddDate(0, 0, 1)
		}

		monthDesc := s.Format("January 2006")
		if s.Month() != endInclusive.Month() || s.Year() != endInclusive.Year() {
			monthDesc = fmt.Sprintf("%s - %s", s.Format("January 2006"), endInclusive.Format("January 2006"))
		}

		isEstimated := endInclusive.After(currentMonthStart) || endInclusive.Equal(currentMonthStart)

		return &DateRange{
			Start:            s,
			EndInclusive:     endInclusive,
			EndExclusiveAWS:  endExclusive,
			BillingMonthDesc: monthDesc,
			IsEstimated:      isEstimated,
		}, nil
	}

	if monthsBack <= 0 {
		monthsBack = 1
	}

	// Calculate previous full months
	// e.g. If now is Oct 28, 2026:
	// currentMonthStart = 2026-10-01
	// endExclusiveAWS = currentMonthStart (2026-10-01)
	// endInclusive = 2026-09-30
	endExclusiveAWS := currentMonthStart
	endInclusive := currentMonthStart.AddDate(0, 0, -1)
	start := currentMonthStart.AddDate(0, -monthsBack, 0)

	monthDesc := start.Format("January 2006")
	if monthsBack > 1 {
		monthDesc = fmt.Sprintf("%s - %s", start.Format("January 2006"), endInclusive.Format("January 2006"))
	}

	return &DateRange{
		Start:            start,
		EndInclusive:     endInclusive,
		EndExclusiveAWS:  endExclusiveAWS,
		BillingMonthDesc: monthDesc,
		IsEstimated:      false, // Historical completed months are finalized
	}, nil
}

// ResolvePreviousPeriod calculates the identical preceding timeframe for Month-over-Month comparison.
func ResolvePreviousPeriod(current *DateRange) *DateRange {
	// Duration in months
	// Count months between start and endExclusiveAWS
	yearDiff := current.EndExclusiveAWS.Year() - current.Start.Year()
	monthDiff := int(current.EndExclusiveAWS.Month()) - int(current.Start.Month()) + (yearDiff * 12)
	if monthDiff <= 0 {
		monthDiff = 1
	}

	prevStart := current.Start.AddDate(0, -monthDiff, 0)
	prevExclusiveAWS := current.Start
	prevInclusive := prevExclusiveAWS.AddDate(0, 0, -1)

	monthDesc := prevStart.Format("January 2006")
	if monthDiff > 1 {
		monthDesc = fmt.Sprintf("%s - %s", prevStart.Format("January 2006"), prevInclusive.Format("January 2006"))
	}

	return &DateRange{
		Start:            prevStart,
		EndInclusive:     prevInclusive,
		EndExclusiveAWS:  prevExclusiveAWS,
		BillingMonthDesc: monthDesc,
		IsEstimated:      false,
	}
}
