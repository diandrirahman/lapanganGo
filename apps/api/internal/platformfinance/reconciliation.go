package platformfinance

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ReconciliationCheckStatus describes the result of one independent check.
// BLOCKED means that a preceding integrity fault made the comparison unsafe;
// it is deliberately different from PASS so callers cannot mistake an
// unevaluated check for clean data.
type ReconciliationCheckStatus string

const (
	ReconciliationPass    ReconciliationCheckStatus = "PASS"
	ReconciliationFail    ReconciliationCheckStatus = "FAIL"
	ReconciliationBlocked ReconciliationCheckStatus = "BLOCKED"
)

type ReconciliationStatus string

const (
	ReconciliationClean          ReconciliationStatus = "CLEAN"
	ReconciliationExceptions     ReconciliationStatus = "EXCEPTIONS"
	ReconciliationCheckOnline                         = "ONLINE_LEDGER_GMV_MATCH"
	ReconciliationCheckSnapshot                       = "PAID_SNAPSHOT_SOURCE_MATCH"
	ReconciliationCheckOffline                        = "OFFLINE_ZERO_COMMISSION"
	ReconciliationCheckRefund                         = "REFUND_EXACT_REVERSAL"
	ReconciliationCheckDuplicate                      = "NO_DUPLICATE_EVENTS"
	ReconciliationCheckRollup                         = "SUMMARY_BREAKDOWN_TREND_MATCH"
	ReconciliationCheckOPEX                           = "OPEX_POSTED_REVERSAL_MATCH"
	ReconciliationCheckActual                         = "ACTUAL_METRICS_UNAVAILABLE"
)

var (
	ErrReconciliationInvalidRange                  = errors.New("invalid reconciliation date range")
	ErrReconciliationNoData                        = errors.New("reconciliation repository returned no snapshot")
	ErrReconciliationActualMetricsProviderRequired = errors.New("reconciliation actual metrics provider is required")
)

// ReconciliationQuery is intentionally explicit. Diagnostics must be
// repeatable and must not depend on the wall clock's current MTD default.
type ReconciliationQuery struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type ReconciliationException struct {
	Metric           string `json:"metric"`
	BucketDate       string `json:"bucket_date"`
	ExpectedCount    int64  `json:"expected_count"`
	ActualCount      int64  `json:"actual_count"`
	DifferenceCount  int64  `json:"difference_count"`
	ExpectedRupiah   int64  `json:"expected_rupiah"`
	ActualRupiah     int64  `json:"actual_rupiah"`
	DifferenceRupiah int64  `json:"difference_rupiah"`
	Reason           string `json:"reason,omitempty"`
}

type ReconciliationCheckResult struct {
	Code             string                    `json:"code"`
	Status           ReconciliationCheckStatus `json:"status"`
	DifferenceCount  int64                     `json:"difference_count"`
	DifferenceRupiah int64                     `json:"difference_rupiah"`
	Exceptions       []ReconciliationException `json:"exceptions,omitempty"`
	Reason           string                    `json:"reason,omitempty"`
}

type ReconciliationReport struct {
	Period   Period                      `json:"period"`
	Timezone string                      `json:"timezone"`
	AsOf     time.Time                   `json:"as_of"`
	Status   ReconciliationStatus        `json:"status"`
	Checks   []ReconciliationCheckResult `json:"checks"`
	Clean    bool                        `json:"clean"`
}

type ReconciliationService interface {
	Reconcile(ctx context.Context, query ReconciliationQuery) (*ReconciliationReport, error)
}

// ReconciliationRepository returns one repeatable-read snapshot. The
// snapshot types are exported so the service can be tested with deterministic
// fault fixtures without requiring a disposable PostgreSQL instance.
type ReconciliationRepository interface {
	LoadReconciliationSnapshot(ctx context.Context, utcStart, utcEndExclusive time.Time) (*ReconciliationSnapshot, error)
}

// ReconciliationActualMetrics is the explicit simulation/live contract used
// by the reconciliation report. Empty or UNAVAILABLE_* values are safe for
// the current simulation phase; any AVAILABLE value must fail the gate.
type ReconciliationActualMetrics struct {
	GatewayCapturedGMV       string
	ActualCommissionRevenue  string
	PaymentProcessingExpense string
	OwnerPayable             string
	PlatformRevenue          string
	TransactionContribution  string
	OperatingResult          string
	GrossTakeRateBPS         string
	NetTakeRateBPS           string

	// The status labels above are not sufficient evidence on their own. These
	// pointers mirror the nullable API values so a provider cannot claim that a
	// metric is unavailable while still supplying an actual amount.
	GatewayCapturedGMVValue       *string
	ActualCommissionRevenueValue  *string
	PaymentProcessingExpenseValue *string
	OwnerPayableValue             *string
	PlatformRevenueValue          *string
	TransactionContributionValue  *string
	OperatingResultValue          *string
	GrossTakeRateBPSValue         *int
	NetTakeRateBPSValue           *int
}

func (m ReconciliationActualMetrics) AllUnavailable() bool {
	return unavailableMetric(m.GatewayCapturedGMV) &&
		unavailableMetric(m.ActualCommissionRevenue) &&
		unavailableMetric(m.PaymentProcessingExpense) &&
		unavailableMetric(m.OwnerPayable) &&
		unavailableMetric(m.PlatformRevenue) &&
		unavailableMetric(m.TransactionContribution) &&
		unavailableMetric(m.OperatingResult) &&
		unavailableMetric(m.GrossTakeRateBPS) &&
		unavailableMetric(m.NetTakeRateBPS) &&
		m.GatewayCapturedGMVValue == nil &&
		m.ActualCommissionRevenueValue == nil &&
		m.PaymentProcessingExpenseValue == nil &&
		m.OwnerPayableValue == nil &&
		m.PlatformRevenueValue == nil &&
		m.TransactionContributionValue == nil &&
		m.OperatingResultValue == nil &&
		m.GrossTakeRateBPSValue == nil &&
		m.NetTakeRateBPSValue == nil
}

func unavailableMetric(value string) bool {
	return strings.HasPrefix(value, "UNAVAILABLE")
}

type ReconciliationActualMetricsProvider interface {
	LoadActualMetrics(ctx context.Context, utcStart, utcEndExclusive time.Time) (ReconciliationActualMetrics, error)
}

// NewReconciliationActualMetricsFromSummary adapts the nullable production
// summary contract to the reconciliation provider contract. Keeping this
// adapter explicit prevents a future DTO field from being silently omitted
// from the simulation/live availability gate.
func NewReconciliationActualMetricsFromSummary(metrics Metrics, availability DataAvailability) ReconciliationActualMetrics {
	return ReconciliationActualMetrics{
		GatewayCapturedGMV:            statusForActualValue(metrics.GatewayCapturedGMV, "UNAVAILABLE_UNTIL_GATEWAY"),
		ActualCommissionRevenue:       statusForActualValue(metrics.ActualCommissionRevenue, "UNAVAILABLE_UNTIL_LIVE"),
		PaymentProcessingExpense:      availability.PaymentProcessingExpense,
		OwnerPayable:                  availability.OwnerPayable,
		PlatformRevenue:               availability.ActualPlatformRevenue,
		TransactionContribution:       statusForActualValue(metrics.TransactionContribution, "UNAVAILABLE_UNTIL_LIVE"),
		OperatingResult:               statusForActualValue(metrics.OperatingResult, "UNAVAILABLE_UNTIL_LIVE"),
		GrossTakeRateBPS:              statusForActualIntValue(metrics.GrossTakeRateBps, "UNAVAILABLE_UNTIL_LIVE"),
		NetTakeRateBPS:                statusForActualIntValue(metrics.NetTakeRateBps, "UNAVAILABLE_UNTIL_LIVE"),
		GatewayCapturedGMVValue:       metrics.GatewayCapturedGMV,
		ActualCommissionRevenueValue:  metrics.ActualCommissionRevenue,
		PaymentProcessingExpenseValue: metrics.PaymentProcessingExpense,
		PlatformRevenueValue:          metrics.PlatformRevenue,
		TransactionContributionValue:  metrics.TransactionContribution,
		OperatingResultValue:          metrics.OperatingResult,
		GrossTakeRateBPSValue:         metrics.GrossTakeRateBps,
		NetTakeRateBPSValue:           metrics.NetTakeRateBps,
	}
}

func statusForActualValue(value *string, unavailable string) string {
	if value == nil {
		return unavailable
	}
	return "AVAILABLE"
}

func statusForActualIntValue(value *int, unavailable string) string {
	if value == nil {
		return unavailable
	}
	return "AVAILABLE"
}

type productionReconciliationActualMetricsProvider struct{}

func NewProductionReconciliationActualMetricsProvider() ReconciliationActualMetricsProvider {
	return productionReconciliationActualMetricsProvider{}
}

func (productionReconciliationActualMetricsProvider) LoadActualMetrics(context.Context, time.Time, time.Time) (ReconciliationActualMetrics, error) {
	metrics, availability := currentPlatformActualMetricsContract()
	return NewReconciliationActualMetricsFromSummary(metrics, availability), nil
}

type ReconciliationSnapshot struct {
	AsOf      time.Time
	CutoverAt time.Time

	SummaryTotals ReconciliationTotals
	LedgerTotals  ReconciliationTotals
	SourceTotals  ReconciliationTotals
	OwnerTotals   []ReconciliationTotals
	VenueTotals   []ReconciliationTotals

	// Bucket facts preserve the Jakarta accounting day for every comparison.
	// Period totals remain useful as report summaries, but must never be the
	// only evidence used to decide PASS because opposite row/day differences
	// can cancel each other.
	SummaryBuckets    []ReconciliationBucketTotals
	LedgerBuckets     []ReconciliationBucketTotals
	SourceBuckets     []ReconciliationBucketTotals
	OwnerBuckets      []ReconciliationBucketTotals
	VenueBuckets      []ReconciliationBucketTotals
	TrendTotals       []ReconciliationBucketTotals
	OPEXSourceBuckets []ReconciliationBucketTotals

	// Events are the canonical online income/refund events used by the public
	// projection read model. They are kept in the snapshot so the service can
	// independently aggregate summary, owner/venue, and trend totals.
	IncomeEvents []ReconciliationEvent
	RefundEvents []ReconciliationEvent

	// PaidBookings carries the source-side snapshot/ledger relationship. It is
	// separate from IncomeEvents so a missing ledger event remains observable.
	PaidBookings []ReconciliationPaidBooking

	OPEXEvents []ReconciliationOPEXEvent
	DataIssues []ReconciliationDataIssue

	// ProjectionFactsIncomplete means an integrity issue prevented the
	// projection event stream from being safely collected. Dependent checks must
	// report BLOCKED instead of treating empty totals as clean.
	ProjectionFactsIncomplete bool
	ActualMetrics             ReconciliationActualMetrics
}

type ReconciliationTotals struct {
	Gross            int64
	Refund           int64
	Net              int64
	Commission       int64
	BookingCount     int64
	RefundCount      int64
	OperatingExpense int64
}

type ReconciliationBucketTotals struct {
	BucketDate string
	Totals     ReconciliationTotals
}

type ReconciliationEvent struct {
	BookingID          string
	OwnerProfileID     string
	VenueID            string
	EventAt            time.Time
	Amount             int64
	Commission         int64
	OriginalAmount     int64
	ExpectedCommission int64
	Exact              bool
	Source             string
}

type ReconciliationPaidBooking struct {
	BookingID          string
	CreatedAt          time.Time
	OwnerProfileID     string
	VenueID            string
	Offline            bool
	Snapshot           bool
	SnapshotSource     string
	CommissionBPS      int32
	Commission         int64
	SnapshotFinalPrice int64
	LedgerCount        int64
	LedgerAmount       int64
}

type ReconciliationOPEXEvent struct {
	ExpenseID      string
	JournalID      string
	EffectiveAt    time.Time
	ExpectedAmount int64
	Amount         int64
	ExactLink      bool
}

type ReconciliationDataIssue struct {
	Code             string
	BucketDate       string
	DifferenceCount  int64
	DifferenceRupiah int64
	Reason           string
}

// reconciliationProjectionRowError keeps the offending event timestamp when
// a row-level projection validation fails. Reconciliation exceptions must be
// dated; an undated fallback would make a report impossible to act on.
type reconciliationProjectionRowError struct {
	At  time.Time
	Err error
}

func (e *reconciliationProjectionRowError) Error() string {
	if e == nil || e.Err == nil {
		return "projection row validation failed"
	}
	return e.Err.Error()
}

func (e *reconciliationProjectionRowError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type reconciliationService struct {
	repo ReconciliationRepository
}

func NewReconciliationService(repo ReconciliationRepository) ReconciliationService {
	return &reconciliationService{repo: repo}
}
