export type FinanceGranularity = 'auto' | 'day' | 'week' | 'month';

export interface PlatformFinanceSummaryQuery {
  start_date?: string;
  end_date?: string;
  owner_profile_id?: string;
  venue_id?: string;
  granularity?: FinanceGranularity;
}

export interface PlatformFinanceMetrics {
  online_gmv_gross: string;
  refund_principal: string;
  online_gmv_net: string;
  projected_commission: string;
  projected_owner_net_after_hypothetical_commission: string;
  realized_online_booking_count: number;
  refunded_booking_count: number;
  legacy_manual_realized_gmv: string;
  gateway_captured_gmv: string | null;
  actual_commission_revenue: string | null;
  payment_processing_expense: string | null;
  platform_operating_expense: string | null;
  projected_operating_result_before_transaction_costs: string | null;
  platform_revenue: string | null;
  transaction_contribution: string | null;
  operating_result: string | null;
}

export interface PlatformFinanceTrendItem {
  period_start: string;
  period_end: string;
  online_gmv_gross: string;
  refund_principal: string;
  online_gmv_net: string;
  projected_commission: string;
  platform_operating_expense: string | null;
}

export interface PlatformFinanceSummaryResponse {
  period: { start_date: string; end_date: string };
  mode: 'SIMULATION' | string;
  currency: 'IDR' | string;
  timezone: 'Asia/Jakarta' | string;
  generated_at: string;
  as_of: string;
  granularity: FinanceGranularity;
  metrics: PlatformFinanceMetrics;
  data_availability: {
    platform_operating_expense: string;
    actual_platform_revenue: string;
    payment_processing_expense: string;
    owner_payable: string;
  };
  trend: PlatformFinanceTrendItem[];
  caveats: string[];
}

export interface PlatformFinanceBreakdownQuery extends PlatformFinanceSummaryQuery {
  dimension: 'owner' | 'venue';
  page?: number;
  limit?: number;
}

export interface PlatformFinanceBreakdownRow {
  owner_profile_id?: string;
  business_name?: string;
  venue_id?: string;
  venue_name?: string;
  realized_online_booking_count: number;
  online_gmv_net: string;
  projected_commission: string;
  projection_basis: string;
  legacy_scenario_count: number;
  snapshot_projection_count: number;
  non_billable_projection_amount: string;
  snapshot_projection_amount: string;
}

export interface PlatformFinanceBreakdownResponse {
  mode: 'SIMULATION' | string;
  data: PlatformFinanceBreakdownRow[];
  total_items: number;
  total_pages: number;
  page: number;
  limit: number;
  as_of: string;
  generated_at: string;
  metric_source_version: string;
  projection_basis: string;
  legacy_scenario_count: number;
  snapshot_projection_count: number;
  non_billable_projection_amount: string;
  snapshot_projection_amount: string;
  platform_operating_expense: string | null;
  data_availability: PlatformFinanceSummaryResponse['data_availability'];
}
