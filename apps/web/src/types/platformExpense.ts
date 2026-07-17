export const EXPENSE_CATEGORIES = [
  'INFRASTRUCTURE',
  'MARKETING',
  'CUSTOMER_SUPPORT',
  'SALARY_CONTRACTOR',
  'LEGAL_COMPLIANCE',
  'PAYMENT_OPERATIONS',
  'OFFICE_ADMIN',
  'OTHER',
] as const;

export const EXPENSE_PAYMENT_ACCOUNTS = ['FUNDING_CLEARING', 'ACCOUNTS_PAYABLE'] as const;
export const EXPENSE_STATUSES = ['DRAFT', 'APPROVED', 'POSTED', 'VOID', 'CANCELLED'] as const;

export type ExpenseCategory = typeof EXPENSE_CATEGORIES[number];
export type ExpensePaymentAccount = typeof EXPENSE_PAYMENT_ACCOUNTS[number];
export type ExpenseStatus = typeof EXPENSE_STATUSES[number];

export interface PlatformExpense {
  id: string;
  category: ExpenseCategory;
  vendor: string | null;
  amount_rupiah: string;
  currency: 'IDR';
  occurred_at: string;
  payment_account: ExpensePaymentAccount;
  external_reference: string | null;
  description: string;
  status: ExpenseStatus;
  posted_journal_id: string | null;
  void_journal_id: string | null;
  created_by_user_id: string;
  approved_by_user_id: string | null;
  posted_by_user_id: string | null;
  voided_by_user_id: string | null;
  cancelled_by_user_id: string | null;
  cancel_reason: string | null;
  void_reason: string | null;
  created_at: string;
  approved_at: string | null;
  posted_at: string | null;
  voided_at: string | null;
  cancelled_at: string | null;
}

export interface PlatformExpenseQuery {
  status?: ExpenseStatus;
  category?: ExpenseCategory;
  page?: number;
  limit?: number;
}

export interface PlatformJournal {
  id: string;
  event_key: string;
  event_type: string;
  booking_id: string | null;
  owner_profile_id: string | null;
  venue_id: string | null;
  currency: string;
  effective_at: string;
  posted_at: string;
  reverses_journal_id: string | null;
  reversal_reason: string | null;
  reversed_by_journal_id: string | null;
  entry_count: number;
  debit_total_rupiah: string;
  credit_total_rupiah: string;
}

export interface PlatformJournalQuery {
  start_date?: string;
  end_date?: string;
  event_type?: string;
  account_code?: string;
  page?: number;
  limit?: number;
}

export interface CreatePlatformExpenseRequest {
  amount_rupiah: string;
  currency: 'IDR';
  occurred_at: string;
  category: ExpenseCategory;
  payment_account: ExpensePaymentAccount;
  vendor?: string;
  external_reference?: string;
  description: string;
}

export interface FinanceApiErrorBody {
  code?: string;
  message?: string;
  field_errors?: Record<string, string>;
}
