package finance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	CreateTransaction(ctx context.Context, tx FinanceTransaction) (FinanceTransaction, error)
	UpdateTransaction(ctx context.Context, id string, ownerID string, req UpdateTransactionRequest) (FinanceTransaction, error)
	DeleteTransaction(ctx context.Context, id string, ownerID string) error
	GetTransactions(ctx context.Context, ownerID string, query TransactionQuery) ([]FinanceTransaction, int, error)
	GetFinanceSummary(ctx context.Context, ownerID string, query FinanceSummaryQuery) (FinanceSummaryResult, error)
	VerifyVenueOwnership(ctx context.Context, venueID string, ownerID string) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) VerifyVenueOwnership(ctx context.Context, venueID string, ownerID string) error {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM venues v
			JOIN owner_profiles op ON v.owner_profile_id = op.id
			WHERE v.id = $1 AND op.user_id = $2
		)
	`
	err := r.db.QueryRow(ctx, query, venueID, ownerID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("forbidden: venue %s does not belong to owner %s", venueID, ownerID)
	}
	return nil
}

func (r *repository) CreateTransaction(ctx context.Context, tx FinanceTransaction) (FinanceTransaction, error) {
	query := `
		INSERT INTO owner_finance_transactions 
		(owner_id, venue_id, type, source, category, amount, transaction_date, payment_method, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query,
		tx.OwnerID, tx.VenueID, tx.Type, tx.Source, tx.Category,
		tx.Amount, tx.TransactionDate, tx.PaymentMethod, tx.Description,
	).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		return tx, err
	}
	return tx, nil
}

func (r *repository) UpdateTransaction(ctx context.Context, id string, ownerID string, req UpdateTransactionRequest) (FinanceTransaction, error) {
	// First fetch the transaction to ensure it's MANUAL and belongs to the owner
	var tx FinanceTransaction
	checkQuery := `
		SELECT id, source FROM owner_finance_transactions 
		WHERE id = $1 AND owner_id = $2
	`
	err := r.db.QueryRow(ctx, checkQuery, id, ownerID).Scan(&tx.ID, &tx.Source)
	if err != nil {
		return tx, err // Not found or unauthorized
	}

	if tx.Source == "BOOKING" {
		return tx, fmt.Errorf("cannot update booking transactions")
	}

	// Update fields
	updateQuery := `
		UPDATE owner_finance_transactions
		SET 
			venue_id = COALESCE($1, venue_id),
			type = COALESCE($2, type),
			category = COALESCE($3, category),
			amount = COALESCE($4, amount),
			transaction_date = COALESCE($5, transaction_date),
			payment_method = COALESCE($6, payment_method),
			description = COALESCE($7, description),
			updated_at = now()
		WHERE id = $8 AND owner_id = $9
		RETURNING 
			id, owner_id, venue_id, booking_id, created_by_user_id, 
			type, source, category, amount, to_char(transaction_date, 'YYYY-MM-DD'), 
			payment_method, description, attachment_url, created_at, updated_at
	`

	err = r.db.QueryRow(ctx, updateQuery,
		req.VenueID, req.Type, req.Category, req.Amount, req.TransactionDate,
		req.PaymentMethod, req.Description, id, ownerID,
	).Scan(
		&tx.ID, &tx.OwnerID, &tx.VenueID, &tx.BookingID, &tx.CreatedByUserID,
		&tx.Type, &tx.Source, &tx.Category, &tx.Amount, &tx.TransactionDate,
		&tx.PaymentMethod, &tx.Description, &tx.AttachmentURL, &tx.CreatedAt, &tx.UpdatedAt,
	)

	if err != nil {
		return tx, err
	}
	return tx, nil
}

func (r *repository) DeleteTransaction(ctx context.Context, id string, ownerID string) error {
	var source string
	checkQuery := `
		SELECT source FROM owner_finance_transactions 
		WHERE id = $1 AND owner_id = $2
	`
	err := r.db.QueryRow(ctx, checkQuery, id, ownerID).Scan(&source)
	if err != nil {
		return err // Not found or unauthorized
	}

	if source == "BOOKING" {
		return fmt.Errorf("cannot delete booking transactions")
	}

	deleteQuery := `DELETE FROM owner_finance_transactions WHERE id = $1 AND owner_id = $2`
	_, err = r.db.Exec(ctx, deleteQuery, id, ownerID)
	return err
}

func (r *repository) GetTransactions(ctx context.Context, ownerID string, q TransactionQuery) ([]FinanceTransaction, int, error) {
	baseQuery := `
		FROM owner_finance_transactions
		WHERE owner_id = $1
	`
	args := []interface{}{ownerID}
	argIdx := 2

	if q.StartDate != "" && q.EndDate != "" {
		baseQuery += fmt.Sprintf(" AND transaction_date >= $%d AND transaction_date <= $%d", argIdx, argIdx+1)
		args = append(args, q.StartDate, q.EndDate)
		argIdx += 2
	} else if q.StartDate != "" {
		baseQuery += fmt.Sprintf(" AND transaction_date >= $%d", argIdx)
		args = append(args, q.StartDate)
		argIdx++
	} else if q.EndDate != "" {
		baseQuery += fmt.Sprintf(" AND transaction_date <= $%d", argIdx)
		args = append(args, q.EndDate)
		argIdx++
	}

	if q.VenueID != "" {
		baseQuery += fmt.Sprintf(" AND venue_id = $%d", argIdx)
		args = append(args, q.VenueID)
		argIdx++
	}

	if q.Type != "" {
		baseQuery += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, q.Type)
		argIdx++
	}

	if q.Category != "" {
		baseQuery += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, q.Category)
		argIdx++
	}

	var total int
	countQuery := "SELECT COUNT(id) " + baseQuery
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	limit := q.Limit
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	selectQuery := `
		SELECT 
			id, owner_id, venue_id, booking_id, created_by_user_id, 
			type, source, category, amount, to_char(transaction_date, 'YYYY-MM-DD'), 
			payment_method, description, attachment_url, created_at, updated_at
	` + baseQuery + fmt.Sprintf(" ORDER BY transaction_date DESC, created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var txs []FinanceTransaction
	for rows.Next() {
		var tx FinanceTransaction
		if err := rows.Scan(
			&tx.ID, &tx.OwnerID, &tx.VenueID, &tx.BookingID, &tx.CreatedByUserID,
			&tx.Type, &tx.Source, &tx.Category, &tx.Amount, &tx.TransactionDate,
			&tx.PaymentMethod, &tx.Description, &tx.AttachmentURL, &tx.CreatedAt, &tx.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		txs = append(txs, tx)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if txs == nil {
		txs = []FinanceTransaction{}
	}

	return txs, total, nil
}

func (r *repository) GetFinanceSummary(ctx context.Context, ownerID string, q FinanceSummaryQuery) (FinanceSummaryResult, error) {
	// A simple implementation of the finance summary combining both ledger and booking data
	// The ledger transactions already contain BOOKING realized revenues.

	res := FinanceSummaryResult{
		VenueBreakdown:    []VenueRevenueItem{},
		StatusBreakdown:   []StatusRevenueItem{},
		DailyCashflow:     []DailyCashflowItem{},
		ExpenseByCategory: []ExpenseCategoryItem{},
	}

	baseQuery := ` FROM owner_finance_transactions WHERE owner_id = $1`
	args := []interface{}{ownerID}
	argIdx := 2

	if q.StartDate != "" && q.EndDate != "" {
		baseQuery += fmt.Sprintf(" AND transaction_date >= $%d AND transaction_date <= $%d", argIdx, argIdx+1)
		args = append(args, q.StartDate, q.EndDate)
		argIdx += 2
	} else if q.StartDate != "" {
		baseQuery += fmt.Sprintf(" AND transaction_date >= $%d", argIdx)
		args = append(args, q.StartDate)
		argIdx++
	} else if q.EndDate != "" {
		baseQuery += fmt.Sprintf(" AND transaction_date <= $%d", argIdx)
		args = append(args, q.EndDate)
		argIdx++
	}

	if q.VenueID != "" {
		baseQuery += fmt.Sprintf(" AND venue_id = $%d", argIdx)
		args = append(args, q.VenueID)
		argIdx++
	}

	// 1. Overall Totals
	totalsQuery := `
		SELECT 
			COALESCE(SUM(CASE WHEN type = 'INCOME' THEN amount ELSE 0 END), 0) as total_income,
			COALESCE(SUM(CASE WHEN type = 'EXPENSE' THEN amount ELSE 0 END), 0) as total_expense,
			COALESCE(SUM(CASE WHEN type = 'INCOME' AND source = 'BOOKING' THEN amount ELSE 0 END), 0) as realized_booking_revenue,
			COALESCE(SUM(CASE WHEN type = 'INCOME' AND source = 'MANUAL' THEN amount ELSE 0 END), 0) as manual_income,
			COALESCE(SUM(CASE WHEN type = 'EXPENSE' AND source = 'MANUAL' THEN amount ELSE 0 END), 0) as manual_expense,
			COALESCE(SUM(CASE WHEN type = 'EXPENSE' AND source = 'REFUND' THEN amount ELSE 0 END), 0) as refund_expense,
			COUNT(id) as transaction_count
		` + baseQuery

	err := r.db.QueryRow(ctx, totalsQuery, args...).Scan(
		&res.TotalIncome, &res.TotalExpense, &res.RealizedBookingRevenue,
		&res.ManualIncome, &res.ManualExpense, &res.RefundExpense, &res.TransactionCount,
	)
	if err != nil {
		return res, err
	}
	res.NetProfit = res.TotalIncome - res.TotalExpense

	// 2. Daily Cashflow
	cashflowQuery := `
		SELECT 
			to_char(transaction_date, 'YYYY-MM-DD') as date,
			COALESCE(SUM(CASE WHEN type = 'INCOME' THEN amount ELSE 0 END), 0) as income,
			COALESCE(SUM(CASE WHEN type = 'EXPENSE' THEN amount ELSE 0 END), 0) as expense
		` + baseQuery + `
		GROUP BY date
		ORDER BY date ASC
	`
	rows, err := r.db.Query(ctx, cashflowQuery, args...)
	if err == nil {
		for rows.Next() {
			var d DailyCashflowItem
			if err := rows.Scan(&d.Date, &d.Income, &d.Expense); err == nil {
				d.Net = d.Income - d.Expense
				res.DailyCashflow = append(res.DailyCashflow, d)
			}
		}
		rows.Close()
	}

	// 3. Expense By Category
	expenseQuery := `
		SELECT 
			category,
			SUM(amount) as amount
		` + baseQuery + ` AND type = 'EXPENSE'
		GROUP BY category
		ORDER BY amount DESC
	`
	rows2, err := r.db.Query(ctx, expenseQuery, args...)
	if err == nil {
		for rows2.Next() {
			var cat ExpenseCategoryItem
			if err := rows2.Scan(&cat.Category, &cat.Amount); err == nil {
				res.ExpenseByCategory = append(res.ExpenseByCategory, cat)
			}
		}
		rows2.Close()
	}

	// 4. Venue Breakdown (Join with venues)
	venueQuery := `
		SELECT 
			v.id as venue_id,
			v.name as venue_name,
			COALESCE(SUM(CASE WHEN t.type = 'INCOME' AND t.source = 'BOOKING' THEN t.amount ELSE 0 END), 0) as realized_revenue,
			COUNT(CASE WHEN t.type = 'INCOME' AND t.source = 'BOOKING' THEN 1 END) as booking_count
		FROM venues v
		JOIN owner_profiles op ON v.owner_profile_id = op.id
		LEFT JOIN owner_finance_transactions t ON v.id = t.venue_id `

	venueArgs := []interface{}{ownerID}
	vArgIdx := 2

	if q.StartDate != "" && q.EndDate != "" {
		venueQuery += fmt.Sprintf(" AND t.transaction_date >= $%d AND t.transaction_date <= $%d", vArgIdx, vArgIdx+1)
		venueArgs = append(venueArgs, q.StartDate, q.EndDate)
		vArgIdx += 2
	} else if q.StartDate != "" {
		venueQuery += fmt.Sprintf(" AND t.transaction_date >= $%d", vArgIdx)
		venueArgs = append(venueArgs, q.StartDate)
		vArgIdx++
	} else if q.EndDate != "" {
		venueQuery += fmt.Sprintf(" AND t.transaction_date <= $%d", vArgIdx)
		venueArgs = append(venueArgs, q.EndDate)
		vArgIdx++
	}

	venueQuery += ` WHERE op.user_id = $1`

	if q.VenueID != "" {
		venueQuery += fmt.Sprintf(" AND v.id = $%d", vArgIdx)
		venueArgs = append(venueArgs, q.VenueID)
		vArgIdx++
	}

	venueQuery += ` GROUP BY v.id, v.name ORDER BY realized_revenue DESC`

	rows3, err := r.db.Query(ctx, venueQuery, venueArgs...)
	if err == nil {
		for rows3.Next() {
			var v VenueRevenueItem
			if err := rows3.Scan(&v.VenueID, &v.VenueName, &v.RealizedRevenue, &v.BookingCount); err == nil {
				res.VenueBreakdown = append(res.VenueBreakdown, v)
			}
		}
		rows3.Close()
	}

	return res, nil
}
