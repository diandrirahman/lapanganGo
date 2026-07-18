package platformfinance_test

import (
	"context"
	"encoding/json"
	"errors"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lapangango-api/internal/platformfinance"
)

type mockRepo struct {
	err error
}

func (m *mockRepo) OwnerMatchesVenue(ctx context.Context, ownerProfileID, venueID string) (bool, error) {
	return true, m.err
}

func (m *mockRepo) GetSummaryData(ctx context.Context, start, end time.Time, ownerID, venueID string) (*platformfinance.SummaryDataResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &platformfinance.SummaryDataResult{}, nil
}

func (m *mockRepo) GetPaginatedBreakdown(ctx context.Context, start, end time.Time, ownerID, venueID, dimension string, page, limit int) (*platformfinance.BreakdownResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &platformfinance.BreakdownResult{Rows: []platformfinance.BreakdownRow{}}, nil
}

func TestHandler_Summary_DuplicateLedger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockRepo{err: platformfinance.ErrDuplicateLedgerDetected}
	svc := platformfinance.NewService(repo)
	handler := platformfinance.NewHandler(svc)

	r := gin.Default()
	r.GET("/summary", handler.GetSummary)

	req, _ := http.NewRequest(http.MethodGet, "/summary?start_date=2026-06-01&end_date=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "DUPLICATE_LEDGER_DETECTED")
}

func TestHandler_Summary_InvalidDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockRepo{err: nil}
	svc := platformfinance.NewService(repo)
	handler := platformfinance.NewHandler(svc)

	r := gin.Default()
	r.GET("/summary", handler.GetSummary)

	req, _ := http.NewRequest(http.MethodGet, "/summary?start_date=2026-06-01&end_date=invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "INVALID_DATE_FORMAT")
	assert.Contains(t, w.Body.String(), "field_errors")
}

func TestHandler_Summary_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockRepo{err: nil}
	svc := platformfinance.NewService(repo)
	handler := platformfinance.NewHandler(svc)

	r := gin.Default()
	r.GET("/summary", handler.GetSummary)

	req, _ := http.NewRequest(http.MethodGet, "/summary?start_date=2026-06-01&end_date=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_Summary_JSONContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := platformfinance.NewHandler(platformfinance.NewService(&mockRepo{err: nil}))
	r := gin.New()
	r.GET("/summary", handler.GetSummary)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/summary?start_date=2026-06-01&end_date=2026-06-30", nil))

	var payload struct {
		Metrics struct {
			PlatformOperatingExpense *string `json:"platform_operating_expense"`
			PlatformRevenue          *string `json:"platform_revenue"`
			TransactionContribution  *string `json:"transaction_contribution"`
			OperatingResult          *string `json:"operating_result"`
		} `json:"metrics"`
		DataAvailability struct {
			PlatformOperatingExpense string `json:"platform_operating_expense"`
			ActualPlatformRevenue    string `json:"actual_platform_revenue"`
		} `json:"data_availability"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "0", *payload.Metrics.PlatformOperatingExpense)
	assert.Equal(t, "AVAILABLE", payload.DataAvailability.PlatformOperatingExpense)
	assert.Equal(t, "UNAVAILABLE_UNTIL_LIVE", payload.DataAvailability.ActualPlatformRevenue)
	assert.Nil(t, payload.Metrics.PlatformRevenue)
	assert.Nil(t, payload.Metrics.TransactionContribution)
	assert.Nil(t, payload.Metrics.OperatingResult)
}

func TestHandler_Summary_SanitizesInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockRepo{err: errors.New(`pq: relation "owner_finance_transactions" does not exist`)}
	handler := platformfinance.NewHandler(platformfinance.NewService(repo))

	r := gin.New()
	r.GET("/summary", handler.GetSummary)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/summary?start_date=2026-06-01&end_date=2026-06-30", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "FINANCE_UNAVAILABLE")
	assert.Contains(t, w.Body.String(), "field_errors")
	assert.NotContains(t, w.Body.String(), "owner_finance_transactions")
}

func TestHandler_Summary_RefundAmountMismatchFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockRepo{err: platformfinance.ErrRefundAmountMismatch}
	handler := platformfinance.NewHandler(platformfinance.NewService(repo))

	r := gin.New()
	r.GET("/summary", handler.GetSummary)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/summary?start_date=2026-06-01&end_date=2026-06-30", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "REFUND_AMOUNT_MISMATCH")
	assert.Contains(t, w.Body.String(), "field_errors")
}
