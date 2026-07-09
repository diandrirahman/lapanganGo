package finance

import (
	"time"
)

type FinanceSummaryQuery struct {
	StartDate string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate   string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
	VenueID         string   `form:"venue_id" binding:"omitempty,uuid"`
	AllowedVenueIDs []string `json:"-"`
}

type TransactionQuery struct {
	StartDate string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate   string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
	VenueID   string `form:"venue_id" binding:"omitempty,uuid"`
	Type      string `form:"type" binding:"omitempty,oneof=INCOME EXPENSE"`
	Category  string `form:"category" binding:"omitempty"`
	Page            int      `form:"page" binding:"omitempty,min=1"`
	Limit           int      `form:"limit" binding:"omitempty,min=1,max=100"`
	AllowedVenueIDs []string `json:"-"`
}

type CreateTransactionRequest struct {
	VenueID         *string `json:"venue_id"`
	Type            string  `json:"type" binding:"required,oneof=INCOME EXPENSE"`
	Category        string  `json:"category" binding:"required"`
	Amount          float64 `json:"amount" binding:"required,gt=0"`
	TransactionDate string  `json:"transaction_date" binding:"required,datetime=2006-01-02"`
	PaymentMethod   *string `json:"payment_method"`
	Description     *string `json:"description"`
}

type UpdateTransactionRequest struct {
	VenueID         *string  `json:"venue_id"`
	Type            *string  `json:"type" binding:"omitempty,oneof=INCOME EXPENSE"`
	Category        *string  `json:"category"`
	Amount          *float64 `json:"amount" binding:"omitempty,gt=0"`
	TransactionDate *string  `json:"transaction_date" binding:"omitempty,datetime=2006-01-02"`
	PaymentMethod   *string  `json:"payment_method"`
	Description     *string  `json:"description"`
}

type FinanceTransaction struct {
	ID              string    `json:"id"`
	OwnerID         string    `json:"owner_id"`
	VenueID         *string   `json:"venue_id"`
	BookingID       *string   `json:"booking_id"`
	CreatedByUserID *string   `json:"created_by_user_id"`
	Type            string    `json:"type"`
	Source          string    `json:"source"`
	Category        string    `json:"category"`
	Amount          float64   `json:"amount"`
	TransactionDate string    `json:"transaction_date"`
	PaymentMethod   *string   `json:"payment_method"`
	Description     *string   `json:"description"`
	AttachmentURL   *string   `json:"attachment_url"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type TransactionListResponse struct {
	Transactions []FinanceTransaction `json:"transactions"`
	Total        int                  `json:"total"`
	Page         int                  `json:"page"`
	Limit        int                  `json:"limit"`
}

type FinanceSummaryResult struct {
	TotalIncome            float64 `json:"total_income"`
	TotalExpense           float64 `json:"total_expense"`
	NetProfit              float64 `json:"net_profit"`
	RealizedBookingRevenue float64 `json:"realized_booking_revenue"`
	ManualIncome           float64 `json:"manual_income"`
	ManualExpense          float64 `json:"manual_expense"`
	RefundExpense          float64 `json:"refund_expense"`
	TransactionCount       int     `json:"transaction_count"`

	VenueBreakdown    []VenueRevenueItem    `json:"venue_breakdown"`
	StatusBreakdown   []StatusRevenueItem   `json:"status_breakdown"`
	DailyCashflow     []DailyCashflowItem   `json:"daily_cashflow"`
	ExpenseByCategory []ExpenseCategoryItem `json:"expense_by_category"`
}

type VenueRevenueItem struct {
	VenueID         string  `json:"venue_id"`
	VenueName       string  `json:"venue_name"`
	RealizedRevenue float64 `json:"realized_revenue"`
	BookingCount    int     `json:"booking_count"`
}

type StatusRevenueItem struct {
	Status       string  `json:"status"`
	Amount       float64 `json:"amount"`
	BookingCount int     `json:"booking_count"`
}

type DailyCashflowItem struct {
	Date    string  `json:"date"`
	Income  float64 `json:"income"`
	Expense float64 `json:"expense"`
	Net     float64 `json:"net"`
}

type ExpenseCategoryItem struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}
