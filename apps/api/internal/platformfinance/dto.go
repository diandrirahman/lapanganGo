package platformfinance

type Period struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type Metrics struct {
	OnlineGMVGross                                 string  `json:"online_gmv_gross"`
	RefundPrincipal                                string  `json:"refund_principal"`
	OnlineGMVNet                                   string  `json:"online_gmv_net"`
	ProjectedCommission                            string  `json:"projected_commission"`
	ProjectedOwnerNetAfterHypotheticalCommission   string  `json:"projected_owner_net_after_hypothetical_commission"`
	ProjectedTakeRateBps                           *int    `json:"projected_take_rate_bps"`
	RealizedOnlineBookingCount                     int     `json:"realized_online_booking_count"`
	RefundedBookingCount                           int     `json:"refunded_booking_count"`
	LegacyManualRealizedGMV                        string  `json:"legacy_manual_realized_gmv"`
	GatewayCapturedGMV                             *string `json:"gateway_captured_gmv"`
	ActualCommissionRevenue                        *string `json:"actual_commission_revenue"`
	PaymentProcessingExpense                       *string `json:"payment_processing_expense"`
	PlatformOperatingExpense                       *string `json:"platform_operating_expense"`
	ProjectedOperatingResultBeforeTransactionCosts *string `json:"projected_operating_result_before_transaction_costs"`
	PlatformRevenue                                *string `json:"platform_revenue"`
	TransactionContribution                        *string `json:"transaction_contribution"`
	OperatingResult                                *string `json:"operating_result"`
	GrossTakeRateBps                               *int    `json:"gross_take_rate_bps"`
	NetTakeRateBps                                 *int    `json:"net_take_rate_bps"`
}

type DataAvailability struct {
	PlatformOperatingExpense string `json:"platform_operating_expense"`
	ActualPlatformRevenue    string `json:"actual_platform_revenue"`
	PaymentProcessingExpense string `json:"payment_processing_expense"`
	OwnerPayable             string `json:"owner_payable"`
}

type DataQuality struct {
	PaidWithoutLedgerCount      int    `json:"paid_without_ledger_count"`
	LedgerWithoutBookingCount   int    `json:"ledger_without_booking_count"`
	LegacyScenarioCount         int    `json:"legacy_scenario_count"`
	SnapshotProjectionCount     int    `json:"snapshot_projection_count"`
	NonBillableProjectionAmount string `json:"non_billable_projection_amount"`
	SnapshotProjectionAmount    string `json:"snapshot_projection_amount"`
	DuplicateLedgerCount        int    `json:"duplicate_ledger_count"`
}

type TrendItem struct {
	PeriodStart                 string  `json:"period_start"`
	PeriodEnd                   string  `json:"period_end"`
	OnlineGMVGross              string  `json:"online_gmv_gross"`
	RefundPrincipal             string  `json:"refund_principal"`
	OnlineGMVNet                string  `json:"online_gmv_net"`
	ProjectedCommission         string  `json:"projected_commission"`
	ProjectionBasis             string  `json:"projection_basis"`
	LegacyScenarioCount         int     `json:"legacy_scenario_count"`
	SnapshotProjectionCount     int     `json:"snapshot_projection_count"`
	NonBillableProjectionAmount string  `json:"non_billable_projection_amount"`
	SnapshotProjectionAmount    string  `json:"snapshot_projection_amount"`
	PlatformOperatingExpense    *string `json:"platform_operating_expense"`
}

type TopOwnerItem struct {
	OwnerProfileID              string `json:"owner_profile_id"`
	BusinessName                string `json:"business_name"`
	RealizedOnlineBookingCount  int    `json:"realized_online_booking_count"`
	OnlineGMVNet                string `json:"online_gmv_net"`
	ProjectedCommission         string `json:"projected_commission"`
	ProjectionBasis             string `json:"projection_basis"`
	LegacyScenarioCount         int    `json:"legacy_scenario_count"`
	SnapshotProjectionCount     int    `json:"snapshot_projection_count"`
	NonBillableProjectionAmount string `json:"non_billable_projection_amount"`
	SnapshotProjectionAmount    string `json:"snapshot_projection_amount"`
}

type TopVenueItem struct {
	VenueID                     string `json:"venue_id"`
	VenueName                   string `json:"venue_name"`
	OwnerProfileID              string `json:"owner_profile_id"`
	RealizedOnlineBookingCount  int    `json:"realized_online_booking_count"`
	OnlineGMVNet                string `json:"online_gmv_net"`
	ProjectedCommission         string `json:"projected_commission"`
	ProjectionBasis             string `json:"projection_basis"`
	LegacyScenarioCount         int    `json:"legacy_scenario_count"`
	SnapshotProjectionCount     int    `json:"snapshot_projection_count"`
	NonBillableProjectionAmount string `json:"non_billable_projection_amount"`
	SnapshotProjectionAmount    string `json:"snapshot_projection_amount"`
}

type SummaryResponse struct {
	Period               Period           `json:"period"`
	Mode                 string           `json:"mode"`
	Currency             string           `json:"currency"`
	Timezone             string           `json:"timezone"`
	GeneratedAt          string           `json:"generated_at"`
	AsOf                 string           `json:"as_of"`
	Granularity          string           `json:"granularity"`
	DefaultCommissionBps int              `json:"default_commission_bps"`
	MetricSourceVersion  string           `json:"metric_source_version"`
	ProjectionBasis      string           `json:"projection_basis"`
	Metrics              Metrics          `json:"metrics"`
	DataAvailability     DataAvailability `json:"data_availability"`
	DataQuality          DataQuality      `json:"data_quality"`
	Trend                []TrendItem      `json:"trend"`
	TopOwnerBreakdown    []TopOwnerItem   `json:"top_owner_breakdown"`
	TopVenueBreakdown    []TopVenueItem   `json:"top_venue_breakdown"`
	Caveats              []string         `json:"caveats"`
}

type PaginatedBreakdownResponse struct {
	Mode                        string `json:"mode"`
	Data                        any    `json:"data"`
	TotalItems                  int    `json:"total_items"`
	TotalPages                  int    `json:"total_pages"`
	Page                        int    `json:"page"`
	Limit                       int    `json:"limit"`
	AsOf                        string `json:"as_of"`
	GeneratedAt                 string `json:"generated_at"`
	MetricSourceVersion         string `json:"metric_source_version"`
	ProjectionBasis             string `json:"projection_basis"`
	LegacyScenarioCount         int    `json:"legacy_scenario_count"`
	SnapshotProjectionCount     int    `json:"snapshot_projection_count"`
	NonBillableProjectionAmount string `json:"non_billable_projection_amount"`
	SnapshotProjectionAmount    string `json:"snapshot_projection_amount"`
}

type FinanceQuery struct {
	StartDate      string `form:"start_date"`
	EndDate        string `form:"end_date"`
	OwnerProfileID string `form:"owner_profile_id" binding:"omitempty,uuid"`
	VenueID        string `form:"venue_id" binding:"omitempty,uuid"`
	Granularity    string `form:"granularity" binding:"omitempty,oneof=auto day week month"`
}

type FinanceBreakdownQuery struct {
	FinanceQuery
	Dimension string `form:"dimension" binding:"required,oneof=owner venue"`
	Page      int    `form:"page" binding:"omitempty,min=1"`
	Limit     int    `form:"limit" binding:"omitempty,min=1,max=100"`
}
