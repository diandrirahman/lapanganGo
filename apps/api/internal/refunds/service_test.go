package refunds_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"lapangango-api/internal/refunds"
)

type mockRepo struct {
	booking         refunds.BookingForRefund
	bookingErr      error
	activeReq       *refunds.RefundRequestResponse
	activeReqErr    error
	latestReq       *refunds.RefundRequestResponse
	latestReqErr    error
	createReq       refunds.RefundRequestResponse
	createReqErr    error
	reqByID         refunds.RefundRequestResponse
	reqByIDErr      error
	tx              pgx.Tx
	txErr           error
	hasIncome       bool
	hasIncomeErr    error
	hasRefund       bool
	hasRefundErr    error
	updateStatusErr error
	insertLedgerErr error
	updateReqErr    error
}

func (m *mockRepo) FindBookingForRefundRequest(ctx context.Context, bookingID string) (refunds.BookingForRefund, error) {
	return m.booking, m.bookingErr
}
func (m *mockRepo) CreateRefundRequest(ctx context.Context, req refunds.RefundRequestResponse) (refunds.RefundRequestResponse, error) {
	return m.createReq, m.createReqErr
}
func (m *mockRepo) GetActiveRefundRequestByBookingID(ctx context.Context, bookingID string) (*refunds.RefundRequestResponse, error) {
	return m.activeReq, m.activeReqErr
}
func (m *mockRepo) GetLatestRefundRequestByBookingID(ctx context.Context, bookingID string) (*refunds.RefundRequestResponse, error) {
	return m.latestReq, m.latestReqErr
}
func (m *mockRepo) GetRefundRequestByID(ctx context.Context, id string) (refunds.RefundRequestResponse, error) {
	return m.reqByID, m.reqByIDErr
}
func (m *mockRepo) ListOwnerRefundRequests(ctx context.Context, ownerID string, status string, venueID string, page, limit int) ([]refunds.OwnerRefundRequestListItem, int, error) {
	return nil, 0, nil
}
func (m *mockRepo) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return m.tx, m.txErr
}
func (m *mockRepo) LockRefundRequest(ctx context.Context, tx pgx.Tx, id string) (refunds.RefundRequestResponse, error) {
	return m.reqByID, m.reqByIDErr
}
func (m *mockRepo) LockBooking(ctx context.Context, tx pgx.Tx, bookingID string) (refunds.BookingForRefund, error) {
	return m.booking, m.bookingErr
}
func (m *mockRepo) HasBookingIncomeLedger(ctx context.Context, tx pgx.Tx, bookingID string) (bool, error) {
	return m.hasIncome, m.hasIncomeErr
}
func (m *mockRepo) HasRefundLedger(ctx context.Context, tx pgx.Tx, bookingID string) (bool, error) {
	return m.hasRefund, m.hasRefundErr
}
func (m *mockRepo) UpdateBookingStatus(ctx context.Context, tx pgx.Tx, bookingID, status string) error {
	return m.updateStatusErr
}
func (m *mockRepo) InsertRefundLedger(ctx context.Context, tx pgx.Tx, ownerID, venueID, bookingID, ownerUserID string, amount float64, description string) error {
	return m.insertLedgerErr
}
func (m *mockRepo) UpdateRefundRequest(ctx context.Context, tx pgx.Tx, id, status, ownerNote, reviewedBy string) error {
	return m.updateReqErr
}

type mockTx struct {
	pgx.Tx
	commitErr   error
	rollbackErr error
}

func (m *mockTx) Commit(ctx context.Context) error {
	return m.commitErr
}
func (m *mockTx) Rollback(ctx context.Context) error {
	return m.rollbackErr
}

func TestRequestBookingRefund(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, loc)
	refunds.SetTimeNow(func() time.Time { return now })
	defer refunds.SetTimeNow(time.Now)

	validReason := "Saya tidak bisa hadir karena jadwal mendadak berubah."

	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{
			booking: refunds.BookingForRefund{
				CustomerID: "c1",
				Status:     "PAID",
				Date:       now,
				StartTime:  now.Add(2 * time.Hour), // 2 hours in future, allowed
			},
			activeReq: nil,
			createReq: refunds.RefundRequestResponse{ID: "r1"},
		}
		service := refunds.NewService(repo, nil)

		req, err := service.RequestBookingRefund(context.Background(), "c1", "b1", validReason)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if req.ID != "r1" {
			t.Errorf("expected r1, got %v", req.ID)
		}
	})

	t.Run("cutoff exceeded", func(t *testing.T) {
		repo := &mockRepo{
			booking: refunds.BookingForRefund{
				CustomerID: "c1",
				Status:     "PAID",
				Date:       now,
				StartTime:  now.Add(30 * time.Minute), // < 1 hour in future, cutoff exceeded
			},
		}
		service := refunds.NewService(repo, nil)

		_, err := service.RequestBookingRefund(context.Background(), "c1", "b1", validReason)
		if err != refunds.ErrBookingRefundCutoffExceeded {
			t.Errorf("expected ErrBookingRefundCutoffExceeded, got %v", err)
		}
	})

	t.Run("not allowed status", func(t *testing.T) {
		repo := &mockRepo{
			booking: refunds.BookingForRefund{
				CustomerID: "c1",
				Status:     "COMPLETED",
				Date:       now,
				StartTime:  now.Add(2 * time.Hour),
			},
		}
		service := refunds.NewService(repo, nil)

		_, err := service.RequestBookingRefund(context.Background(), "c1", "b1", validReason)
		if err != refunds.ErrRefundRequestNotAllowed {
			t.Errorf("expected ErrRefundRequestNotAllowed, got %v", err)
		}
	})
}

func TestApproveRefundRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{
			tx: &mockTx{},
			reqByID: refunds.RefundRequestResponse{
				OwnerID: "o1",
				Status:  "PENDING",
			},
			booking: refunds.BookingForRefund{
				Status: "PAID",
			},
			hasIncome: true,
			hasRefund: false,
		}
		service := refunds.NewService(repo, nil)

		_, err := service.ApproveRefundRequest(context.Background(), "o1", "r1", "ok")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("already reviewed", func(t *testing.T) {
		repo := &mockRepo{
			tx: &mockTx{},
			reqByID: refunds.RefundRequestResponse{
				OwnerID: "o1",
				Status:  "APPROVED",
			},
		}
		service := refunds.NewService(repo, nil)

		_, err := service.ApproveRefundRequest(context.Background(), "o1", "r1", "ok")
		if err != refunds.ErrRefundRequestAlreadyReviewed {
			t.Errorf("expected ErrRefundRequestAlreadyReviewed, got %v", err)
		}
	})
}
