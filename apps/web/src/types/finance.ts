export interface VenueRevenueItem {
  venue_id: string;
  venue_name: string;
  realized_revenue: number;
  booking_count: number;
}

export interface StatusRevenueItem {
  status: string;
  amount: number;
  booking_count: number;
}

export interface DailyCashflowItem {
  date: string;
  income: number;
  expense: number;
  net: number;
}

export interface ExpenseCategoryItem {
  category: string;
  amount: number;
}

export interface FinanceSummaryResult {
  total_income: number;
  total_expense: number;
  net_profit: number;
  realized_booking_revenue: number;
  manual_income: number;
  manual_expense: number;
  refund_expense: number;
  transaction_count: number;
  venue_breakdown: VenueRevenueItem[];
  status_breakdown: StatusRevenueItem[];
  daily_cashflow: DailyCashflowItem[];
  expense_by_category: ExpenseCategoryItem[];
}

export interface FinanceTransaction {
  id: string;
  owner_id: string;
  venue_id?: string;
  booking_id?: string;
  created_by_user_id?: string;
  type: 'INCOME' | 'EXPENSE';
  source: 'BOOKING' | 'MANUAL' | 'REFUND' | 'PAYROLL' | 'MAINTENANCE' | 'OTHER';
  category: string;
  amount: number;
  transaction_date: string;
  payment_method?: string;
  description?: string;
  attachment_url?: string;
  created_at: string;
  updated_at: string;
}

export interface TransactionListResponse {
  transactions: FinanceTransaction[];
  total: number;
  page: number;
  limit: number;
}

export interface CreateTransactionRequest {
  venue_id?: string;
  type: 'INCOME' | 'EXPENSE';
  category: string;
  amount: number;
  transaction_date: string;
  payment_method?: string;
  description?: string;
}
