package platformfinance

import (
	"context"
	"math/big"
	"strconv"
	"time"
)

type Service interface {
	GetSummary(ctx context.Context, query FinanceQuery) (*SummaryResponse, error)
	GetBreakdown(ctx context.Context, query FinanceBreakdownQuery) (*PaginatedBreakdownResponse, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) GetSummary(ctx context.Context, query FinanceQuery) (*SummaryResponse, error) {
	utcStart, utcEndExclusive, err := ParseAndValidateDates(query.StartDate, query.EndDate)
	if err != nil {
		return nil, err
	}

	granularity := DetermineGranularity(utcStart, utcEndExclusive, query.Granularity)
	if err := s.ensureOwnerVenueMatch(ctx, query.OwnerProfileID, query.VenueID); err != nil {
		return nil, err
	}

	data, err := s.repo.GetSummaryData(ctx, utcStart, utcEndExclusive, query.OwnerProfileID, query.VenueID)
	if err != nil {
		return nil, err
	}

	// Use the exact validated query boundaries for response labels. This avoids
	// an MTD request changing labels if the clock crosses midnight mid-request.
	startDateStr := utcStart.In(jakartaLocation).Format("2006-01-02")
	endDateStr := utcEndExclusive.In(jakartaLocation).AddDate(0, 0, -1).Format("2006-01-02")

	netGMV := data.Gross - data.RefundPrincipal
	netComm := data.ProjectedCommGross - data.ProjectedCommRefunded
	ownerNetAfterComm := netGMV - netComm
	legacyCount := data.LegacyScenarioCount
	snapshotCount := data.SnapshotProjectionCount
	legacyAmount := data.LegacyProjectionAmount
	snapshotAmount := data.SnapshotProjectionAmount
	legacyGross := data.LegacyGross
	projectionBasis := data.ProjectionBasis
	hasProjectionEvents := data.RealizedBookingCount > 0 || data.RefundedBookingCount > 0
	if projectionBasis == "" && hasProjectionEvents {
		return nil, ErrProjectionIntegrity
	}
	if projectionBasis == "" {
		projectionBasis = projectionBasisForEmptyRange(utcStart, utcEndExclusive, data.CutoverAt)
	}

	var bpsValue *int
	if netGMV > 0 {
		v, err := calculateBps(netComm, netGMV)
		if err != nil {
			return nil, err
		}
		bpsValue = &v
	}

	metrics := Metrics{
		OnlineGMVGross:      strconv.FormatInt(data.Gross, 10),
		RefundPrincipal:     strconv.FormatInt(data.RefundPrincipal, 10),
		OnlineGMVNet:        strconv.FormatInt(netGMV, 10),
		ProjectedCommission: strconv.FormatInt(netComm, 10),
		ProjectedOwnerNetAfterHypotheticalCommission:   strconv.FormatInt(ownerNetAfterComm, 10),
		ProjectedTakeRateBps:                           bpsValue,
		RealizedOnlineBookingCount:                     data.RealizedBookingCount,
		RefundedBookingCount:                           data.RefundedBookingCount,
		LegacyManualRealizedGMV:                        strconv.FormatInt(legacyGross, 10),
		GatewayCapturedGMV:                             nil,
		ActualCommissionRevenue:                        nil,
		PaymentProcessingExpense:                       nil,
		PlatformOperatingExpense:                       nil,
		ProjectedOperatingResultBeforeTransactionCosts: nil,
		PlatformRevenue:                                nil,
		TransactionContribution:                        nil,
		OperatingResult:                                nil,
		GrossTakeRateBps:                               nil,
		NetTakeRateBps:                                 nil,
	}

	da := DataAvailability{
		PlatformOperatingExpense: "PENDING_PHASE_3B",
		ActualPlatformRevenue:    "UNAVAILABLE_UNTIL_LIVE",
		PaymentProcessingExpense: "UNAVAILABLE_UNTIL_GATEWAY",
		OwnerPayable:             "UNAVAILABLE_UNTIL_PLATFORM_COLLECTED",
	}

	dq := DataQuality{
		PaidWithoutLedgerCount:      data.PaidWithoutLedgerCount,
		LedgerWithoutBookingCount:   data.LedgerWithoutBookingCount,
		LegacyScenarioCount:         legacyCount,
		SnapshotProjectionCount:     snapshotCount,
		NonBillableProjectionAmount: strconv.FormatInt(legacyAmount, 10),
		SnapshotProjectionAmount:    strconv.FormatInt(snapshotAmount, 10),
		DuplicateLedgerCount:        0,
	}

	generatedAt := time.Now().In(jakartaLocation).Format(time.RFC3339)

	// Build trend
	trend := buildContinuousBucketsAt(utcStart, utcEndExclusive, granularity, data.IncomeBuckets, data.RefundBuckets, data.CutoverAt)

	// Build top owners
	topOwners := make([]TopOwnerItem, 0, len(data.TopOwners))
	for _, row := range data.TopOwners {
		topOwners = append(topOwners, TopOwnerItem{
			OwnerProfileID:              row.ID,
			BusinessName:                row.Name,
			RealizedOnlineBookingCount:  row.BookingCount,
			OnlineGMVNet:                strconv.FormatInt(row.Net, 10),
			ProjectedCommission:         strconv.FormatInt(row.NetComm, 10),
			ProjectionBasis:             row.ProjectionBasis,
			LegacyScenarioCount:         row.LegacyScenarioCount,
			SnapshotProjectionCount:     row.SnapshotProjectionCount,
			NonBillableProjectionAmount: strconv.FormatInt(row.NonBillableProjectionAmount, 10),
			SnapshotProjectionAmount:    strconv.FormatInt(row.SnapshotProjectionAmount, 10),
		})
	}
	// Build top venues
	topVenues := make([]TopVenueItem, 0, len(data.TopVenues))
	for _, row := range data.TopVenues {
		topVenues = append(topVenues, TopVenueItem{
			VenueID:                     row.ID,
			VenueName:                   row.Name,
			OwnerProfileID:              row.OwnerProfileID,
			RealizedOnlineBookingCount:  row.BookingCount,
			OnlineGMVNet:                strconv.FormatInt(row.Net, 10),
			ProjectedCommission:         strconv.FormatInt(row.NetComm, 10),
			ProjectionBasis:             row.ProjectionBasis,
			LegacyScenarioCount:         row.LegacyScenarioCount,
			SnapshotProjectionCount:     row.SnapshotProjectionCount,
			NonBillableProjectionAmount: strconv.FormatInt(row.NonBillableProjectionAmount, 10),
			SnapshotProjectionAmount:    strconv.FormatInt(row.SnapshotProjectionAmount, 10),
		})
	}

	res := &SummaryResponse{
		Period:               Period{StartDate: startDateStr, EndDate: endDateStr},
		Mode:                 "SIMULATION",
		Currency:             "IDR",
		Timezone:             "Asia/Jakarta",
		GeneratedAt:          generatedAt,
		AsOf:                 data.AsOf.In(jakartaLocation).Format(time.RFC3339),
		Granularity:          granularity,
		DefaultCommissionBps: 700,
		MetricSourceVersion:  ProjectionMetricVersion,
		ProjectionBasis:      projectionBasis,
		Metrics:              metrics,
		DataAvailability:     da,
		DataQuality:          dq,
		Trend:                trend,
		TopOwnerBreakdown:    topOwners,
		TopVenueBreakdown:    topVenues,
		Caveats:              []string{"Proyeksi komisi bukan pendapatan aktual dan belum ditagihkan kepada owner.", "LEGACY_NO_COMMISSION ditampilkan sebagai skenario historis 7% non-billable; POLICY memakai nilai booking_fee_snapshots."},
	}

	return res, nil
}

func (s *service) ensureOwnerVenueMatch(ctx context.Context, ownerProfileID, venueID string) error {
	if ownerProfileID == "" || venueID == "" {
		return nil
	}
	matches, err := s.repo.OwnerMatchesVenue(ctx, ownerProfileID, venueID)
	if err != nil {
		return err
	}
	if !matches {
		return ErrOwnerVenueMismatch
	}
	return nil
}

// calculateBps returns rounded (half-away-from-zero) basis points without
// passing finance values through binary floating point or overflowing int64.
func calculateBps(numerator, denominator int64) (int, error) {
	if denominator <= 0 {
		return 0, ErrOverflowDetected
	}

	var scaled, quotient, remainder, absR, twiceR big.Int
	scaled.Mul(big.NewInt(numerator), big.NewInt(10_000))
	quotient.QuoRem(&scaled, big.NewInt(denominator), &remainder)
	if remainder.Sign() != 0 {
		absR.Abs(&remainder)
		twiceR.Mul(&absR, big.NewInt(2))
		if twiceR.Cmp(big.NewInt(denominator)) >= 0 {
			if numerator < 0 {
				quotient.Sub(&quotient, big.NewInt(1))
			} else {
				quotient.Add(&quotient, big.NewInt(1))
			}
		}
	}
	if !quotient.IsInt64() {
		return 0, ErrOverflowDetected
	}
	v := quotient.Int64()
	if int64(int(v)) != v {
		return 0, ErrOverflowDetected
	}
	return int(v), nil
}

func maxProjectionTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}

func buildContinuousBuckets(utcStart, utcEndExclusive time.Time, granularity string, income, refund []BucketResult) []TrendItem {
	return buildContinuousBucketsAt(utcStart, utcEndExclusive, granularity, income, refund, time.Time{})
}

func buildContinuousBucketsAt(utcStart, utcEndExclusive time.Time, granularity string, income, refund []BucketResult, cutover time.Time) []TrendItem {
	// Aggregate bucket days into requested granularity
	// Map data by day
	dayGross := make(map[string]int64)
	dayCommGross := make(map[string]int64)
	for _, b := range income {
		dStr := b.Bucket.In(jakartaLocation).Format("2006-01-02")
		dayGross[dStr] += b.Amount
		dayCommGross[dStr] += b.Comm
	}
	dayRefund := make(map[string]int64)
	dayCommRefund := make(map[string]int64)
	dayLegacyCount := make(map[string]int)
	daySnapshotCount := make(map[string]int)
	dayLegacyComm := make(map[string]int64)
	daySnapshotComm := make(map[string]int64)
	dayLegacyRefundComm := make(map[string]int64)
	daySnapshotRefundComm := make(map[string]int64)
	dayLegacyPresent := make(map[string]bool)
	daySnapshotPresent := make(map[string]bool)
	for _, b := range refund {
		dStr := b.Bucket.In(jakartaLocation).Format("2006-01-02")
		dayRefund[dStr] += b.Amount
		dayCommRefund[dStr] += b.Comm
		if b.Source == ProjectionBasisSnapshot {
			daySnapshotPresent[dStr] = true
			daySnapshotRefundComm[dStr] += b.Comm
		} else {
			dayLegacyPresent[dStr] = true
			dayLegacyRefundComm[dStr] += b.Comm
		}
	}
	for _, b := range income {
		dStr := b.Bucket.In(jakartaLocation).Format("2006-01-02")
		if b.Source == ProjectionBasisSnapshot {
			daySnapshotPresent[dStr] = true
			daySnapshotCount[dStr]++
			daySnapshotComm[dStr] += b.Comm
		} else {
			dayLegacyPresent[dStr] = true
			dayLegacyCount[dStr]++
			dayLegacyComm[dStr] += b.Comm
		}
	}

	var trend []TrendItem

	// Start in Jakarta timezone
	curr := utcStart.In(jakartaLocation)
	endWIB := utcEndExclusive.In(jakartaLocation)

	if granularity == "day" {
		for curr.Before(endWIB) {
			dStr := curr.Format("2006-01-02")
			gross := dayGross[dStr]
			ref := dayRefund[dStr]
			cGross := dayCommGross[dStr]
			cRef := dayCommRefund[dStr]

			trend = append(trend, TrendItem{
				PeriodStart:              dStr,
				PeriodEnd:                dStr, // same for day bucket
				OnlineGMVGross:           strconv.FormatInt(gross, 10),
				RefundPrincipal:          strconv.FormatInt(ref, 10),
				OnlineGMVNet:             strconv.FormatInt(gross-ref, 10),
				ProjectedCommission:      strconv.FormatInt(cGross-cRef, 10),
				PlatformOperatingExpense: nil,
			})
			curr = curr.AddDate(0, 0, 1)
		}
	} else if granularity == "week" {
		// Calendar weeks are always Monday--Sunday WIB. Values only include the
		// requested interval, but bucket labels retain the calendar boundary.
		for curr.Weekday() != time.Monday {
			curr = curr.AddDate(0, 0, -1)
		}

		for curr.Before(endWIB) {
			pStart := curr
			pEnd := curr
			for pEnd.Weekday() != time.Sunday {
				pEnd = pEnd.AddDate(0, 0, 1)
			}

			iterStart := pStart
			if iterStart.Before(utcStart.In(jakartaLocation)) {
				iterStart = utcStart.In(jakartaLocation)
			}
			iterEnd := pEnd
			if iterEnd.After(endWIB.AddDate(0, 0, -1)) {
				iterEnd = endWIB.AddDate(0, 0, -1)
			}

			var g, r, cg, cr int64
			iter := iterStart
			for !iter.After(iterEnd) {
				dStr := iter.Format("2006-01-02")
				g += dayGross[dStr]
				r += dayRefund[dStr]
				cg += dayCommGross[dStr]
				cr += dayCommRefund[dStr]
				iter = iter.AddDate(0, 0, 1)
			}

			trend = append(trend, TrendItem{
				PeriodStart:              pStart.Format("2006-01-02"),
				PeriodEnd:                pEnd.Format("2006-01-02"),
				OnlineGMVGross:           strconv.FormatInt(g, 10),
				RefundPrincipal:          strconv.FormatInt(r, 10),
				OnlineGMVNet:             strconv.FormatInt(g-r, 10),
				ProjectedCommission:      strconv.FormatInt(cg-cr, 10),
				PlatformOperatingExpense: nil,
			})
			curr = pStart.AddDate(0, 0, 7)
		}
	} else {
		// Calendar months retain their first/last-day labels while values are
		// restricted to the requested interval.
		for curr.Before(endWIB) {
			pStart := time.Date(curr.Year(), curr.Month(), 1, 0, 0, 0, 0, jakartaLocation)
			pEnd := pStart.AddDate(0, 1, -1) // Last day of month
			iterStart := pStart
			if iterStart.Before(utcStart.In(jakartaLocation)) {
				iterStart = utcStart.In(jakartaLocation)
			}
			iterEnd := pEnd
			if iterEnd.After(endWIB.AddDate(0, 0, -1)) {
				iterEnd = endWIB.AddDate(0, 0, -1)
			}

			var g, r, cg, cr int64
			iter := iterStart
			for !iter.After(iterEnd) {
				dStr := iter.Format("2006-01-02")
				g += dayGross[dStr]
				r += dayRefund[dStr]
				cg += dayCommGross[dStr]
				cr += dayCommRefund[dStr]
				iter = iter.AddDate(0, 0, 1)
			}

			trend = append(trend, TrendItem{
				PeriodStart:              pStart.Format("2006-01-02"),
				PeriodEnd:                pEnd.Format("2006-01-02"),
				OnlineGMVGross:           strconv.FormatInt(g, 10),
				RefundPrincipal:          strconv.FormatInt(r, 10),
				OnlineGMVNet:             strconv.FormatInt(g-r, 10),
				ProjectedCommission:      strconv.FormatInt(cg-cr, 10),
				PlatformOperatingExpense: nil,
			})
			curr = pStart.AddDate(0, 1, 0)
		}
	}

	if len(trend) == 0 {
		// return empty array not null
		trend = []TrendItem{}
	}
	for i := range trend {
		bucketStart, errStart := time.ParseInLocation("2006-01-02", trend[i].PeriodStart, jakartaLocation)
		bucketEnd, errEnd := time.ParseInLocation("2006-01-02", trend[i].PeriodEnd, jakartaLocation)
		if errStart != nil || errEnd != nil {
			continue
		}
		requestStart := utcStart.In(jakartaLocation)
		requestEndExclusive := utcEndExclusive.In(jakartaLocation)
		bucketStart = maxProjectionTime(bucketStart, requestStart)
		bucketEndExclusive := bucketEnd.AddDate(0, 0, 1)
		if bucketEndExclusive.After(requestEndExclusive) {
			bucketEndExclusive = requestEndExclusive
		}
		legacyCount, snapshotCount := 0, 0
		legacyPresent, snapshotPresent := false, false
		legacyComm, snapshotComm := int64(0), int64(0)
		for day := bucketStart; day.Before(bucketEndExclusive); day = day.AddDate(0, 0, 1) {
			key := day.Format("2006-01-02")
			legacyCount += dayLegacyCount[key]
			snapshotCount += daySnapshotCount[key]
			legacyPresent = legacyPresent || dayLegacyPresent[key]
			snapshotPresent = snapshotPresent || daySnapshotPresent[key]
			legacyComm += dayLegacyComm[key] - dayLegacyRefundComm[key]
			snapshotComm += daySnapshotComm[key] - daySnapshotRefundComm[key]
		}
		trend[i].ProjectionBasis = projectionBasisWithPresence(legacyCount, snapshotCount, legacyPresent, snapshotPresent, projectionBasisForEmptyRange(bucketStart.UTC(), bucketEndExclusive.UTC(), cutover))
		trend[i].LegacyScenarioCount = legacyCount
		trend[i].SnapshotProjectionCount = snapshotCount
		trend[i].NonBillableProjectionAmount = strconv.FormatInt(legacyComm, 10)
		trend[i].SnapshotProjectionAmount = strconv.FormatInt(snapshotComm, 10)
	}

	return trend
}

func (s *service) GetBreakdown(ctx context.Context, query FinanceBreakdownQuery) (*PaginatedBreakdownResponse, error) {
	utcStart, utcEndExclusive, err := ParseAndValidateDates(query.StartDate, query.EndDate)
	if err != nil {
		return nil, err
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	limit := query.Limit
	if limit < 1 {
		limit = 20
	}
	if err := s.ensureOwnerVenueMatch(ctx, query.OwnerProfileID, query.VenueID); err != nil {
		return nil, err
	}

	data, err := s.repo.GetPaginatedBreakdown(ctx, utcStart, utcEndExclusive, query.OwnerProfileID, query.VenueID, query.Dimension, page, limit)
	if err != nil {
		return nil, err
	}

	var items []any
	if query.Dimension == "owner" {
		for _, row := range data.Rows {
			items = append(items, TopOwnerItem{
				OwnerProfileID:              row.ID,
				BusinessName:                row.Name,
				RealizedOnlineBookingCount:  row.BookingCount,
				OnlineGMVNet:                strconv.FormatInt(row.Net, 10),
				ProjectedCommission:         strconv.FormatInt(row.NetComm, 10),
				ProjectionBasis:             row.ProjectionBasis,
				LegacyScenarioCount:         row.LegacyScenarioCount,
				SnapshotProjectionCount:     row.SnapshotProjectionCount,
				NonBillableProjectionAmount: strconv.FormatInt(row.NonBillableProjectionAmount, 10),
				SnapshotProjectionAmount:    strconv.FormatInt(row.SnapshotProjectionAmount, 10),
			})
		}
	} else {
		for _, row := range data.Rows {
			items = append(items, TopVenueItem{
				VenueID:                     row.ID,
				VenueName:                   row.Name,
				OwnerProfileID:              row.OwnerProfileID,
				RealizedOnlineBookingCount:  row.BookingCount,
				OnlineGMVNet:                strconv.FormatInt(row.Net, 10),
				ProjectedCommission:         strconv.FormatInt(row.NetComm, 10),
				ProjectionBasis:             row.ProjectionBasis,
				LegacyScenarioCount:         row.LegacyScenarioCount,
				SnapshotProjectionCount:     row.SnapshotProjectionCount,
				NonBillableProjectionAmount: strconv.FormatInt(row.NonBillableProjectionAmount, 10),
				SnapshotProjectionAmount:    strconv.FormatInt(row.SnapshotProjectionAmount, 10),
			})
		}
	}

	if items == nil {
		items = make([]any, 0) // ensuring not null JSON
	}

	totalPages := 0
	if data.TotalItems > 0 {
		totalPages = (data.TotalItems-1)/limit + 1
	}

	return &PaginatedBreakdownResponse{
		Mode:                        "SIMULATION",
		Data:                        items,
		TotalItems:                  data.TotalItems,
		TotalPages:                  totalPages,
		Page:                        page,
		Limit:                       limit,
		AsOf:                        data.AsOf.In(jakartaLocation).Format(time.RFC3339),
		GeneratedAt:                 time.Now().In(jakartaLocation).Format(time.RFC3339),
		MetricSourceVersion:         ProjectionMetricVersion,
		ProjectionBasis:             data.ProjectionBasis,
		LegacyScenarioCount:         data.LegacyScenarioCount,
		SnapshotProjectionCount:     data.SnapshotProjectionCount,
		NonBillableProjectionAmount: strconv.FormatInt(data.NonBillableProjectionAmount, 10),
		SnapshotProjectionAmount:    strconv.FormatInt(data.SnapshotProjectionAmount, 10),
	}, nil
}
