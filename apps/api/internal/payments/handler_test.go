package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestCreatePaymentAttemptRejectsUnknownClientFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(nil)
	router.POST("/bookings/:id/payment-attempts", func(c *gin.Context) {
		c.Set("auth_user_id", uuid.NewString())
		c.Next()
	}, handler.CreatePaymentAttempt)

	req := httptest.NewRequest(http.MethodPost, "/bookings/"+uuid.NewString()+"/payment-attempts", strings.NewReader(`{"requested_method":"QRIS","amount_rupiah":10000}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "client-1")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d; want 400", res.Code)
	}
}

func TestCreatePaymentAttemptRejectsDuplicateRequestedMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(nil)
	router.POST("/bookings/:id/payment-attempts", func(c *gin.Context) {
		c.Set("auth_user_id", uuid.NewString())
		c.Next()
	}, handler.CreatePaymentAttempt)

	req := httptest.NewRequest(
		http.MethodPost,
		"/bookings/"+uuid.NewString()+"/payment-attempts",
		strings.NewReader(`{"requested_method":"QRIS","requested_method":"CARD"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "client-duplicate-field")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("duplicate field status = %d; want 400", res.Code)
	}
}

func TestCreatePaymentAttemptRejectsAmbiguousIdempotencyHeadersBeforeOrchestration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		values []string
	}{
		{name: "missing"},
		{name: "multiple fields", values: []string{"request-one", "request-two"}},
		{name: "proxy-combined", values: []string{"request-one,request-two"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			handler := NewHandler(&stubPaymentOrchestration{
				createPayment: func(context.Context, string, string, string, CreateAttemptRequest) (CreatePaymentResult, error) {
					t.Fatal("ambiguous idempotency header reached orchestration")
					return CreatePaymentResult{}, nil
				},
			})
			router.POST("/bookings/:id/payment-attempts", func(c *gin.Context) {
				c.Set("auth_user_id", uuid.NewString())
				c.Next()
			}, handler.CreatePaymentAttempt)

			req := httptest.NewRequest(
				http.MethodPost,
				"/bookings/"+uuid.NewString()+"/payment-attempts",
				strings.NewReader(`{"requested_method":"QRIS"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			for _, value := range test.values {
				req.Header.Add("Idempotency-Key", value)
			}
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("ambiguous idempotency status = %d body=%s; want 400", res.Code, res.Body.String())
			}
		})
	}
}

func TestCreatePaymentAttemptRejectsTrailingJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(nil)
	router.POST("/bookings/:id/payment-attempts", func(c *gin.Context) {
		c.Set("auth_user_id", uuid.NewString())
		c.Next()
	}, handler.CreatePaymentAttempt)

	req := httptest.NewRequest(http.MethodPost, "/bookings/"+uuid.NewString()+"/payment-attempts", strings.NewReader(`{"requested_method":"QRIS"}{"requested_method":"CARD"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "client-1")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status = %d; want 400", res.Code)
	}
}

func TestCreatePaymentAttemptRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(nil)
	router.POST("/bookings/:id/payment-attempts", func(c *gin.Context) {
		c.Set("auth_user_id", uuid.NewString())
		c.Next()
	}, handler.CreatePaymentAttempt)

	body := `{"requested_method":"` + strings.Repeat("Q", int(maxCreatePaymentRequestBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/bookings/"+uuid.NewString()+"/payment-attempts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "client-oversized")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status = %d body=%s; want 413", res.Code, res.Body.String())
	}
}

func TestCreatePaymentAttemptHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	customerID := uuid.NewString()
	bookingID := uuid.NewString()
	attemptID := uuid.NewString()
	expiresAt := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	var receivedCustomerID, receivedBookingID, receivedKey string
	var receivedRequest CreateAttemptRequest
	orchestration := &stubPaymentOrchestration{
		createPayment: func(_ context.Context, customer, booking, key string, req CreateAttemptRequest) (CreatePaymentResult, error) {
			receivedCustomerID, receivedBookingID, receivedKey, receivedRequest = customer, booking, key, req
			return CreatePaymentResult{
				Attempt: PaymentAttempt{
					ID:          attemptID,
					BookingID:   bookingID,
					State:       AttemptStateCreated,
					ExpiresAt:   expiresAt,
					RequestHash: strings.Repeat("a", 64),
				},
				Replay: true,
			}, nil
		},
	}
	router := gin.New()
	handler := NewHandler(orchestration)
	router.POST("/bookings/:id/payment-attempts", authenticatedTestCustomer(customerID), handler.CreatePaymentAttempt)

	req := httptest.NewRequest(http.MethodPost, "/bookings/"+bookingID+"/payment-attempts", strings.NewReader(`{"requested_method":"QRIS"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "http-contract-replay")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s; want 202", res.Code, res.Body.String())
	}
	if receivedCustomerID != customerID || receivedBookingID != bookingID ||
		receivedKey != "http-contract-replay" || receivedRequest.RequestedMethod != RequestedMethodQRIS {
		t.Fatalf("unexpected orchestration input: customer=%q booking=%q key=%q request=%#v", receivedCustomerID, receivedBookingID, receivedKey, receivedRequest)
	}
	var body struct {
		PaymentAttempt struct {
			ID        string       `json:"id"`
			BookingID string       `json:"booking_id"`
			State     AttemptState `json:"state"`
			ExpiresAt time.Time    `json:"expires_at"`
		} `json:"payment_attempt"`
		StatusURL string `json:"status_url"`
		Replayed  bool   `json:"replayed"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if body.PaymentAttempt.ID != attemptID || body.PaymentAttempt.BookingID != bookingID ||
		body.PaymentAttempt.State != AttemptStateCreated || !body.PaymentAttempt.ExpiresAt.Equal(expiresAt) ||
		body.StatusURL != "/payment-attempts/"+attemptID || !body.Replayed {
		t.Fatalf("unexpected create response: %#v", body)
	}
}

func TestGetPaymentAttemptHTTPContractAndOwnershipDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	customerID := uuid.NewString()
	bookingID := uuid.NewString()
	allowedAttemptID := uuid.NewString()
	deniedAttemptID := uuid.NewString()
	expiresAt := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	checkoutURL := "https://checkout-staging.xendit.co/sessions/ps-661f87c614802d6c402cd82d0"
	orchestration := &stubPaymentOrchestration{
		getPayment: func(_ context.Context, customer, attempt string) (PaymentAttemptView, error) {
			if customer != customerID {
				t.Fatalf("get customer = %q; want %q", customer, customerID)
			}
			if attempt == deniedAttemptID {
				return PaymentAttemptView{}, ErrPaymentAccessDenied
			}
			return PaymentAttemptView{
				ID:          allowedAttemptID,
				BookingID:   bookingID,
				State:       AttemptStatePending,
				ExpiresAt:   expiresAt,
				CheckoutURL: &checkoutURL,
			}, nil
		},
	}
	router := gin.New()
	handler := NewHandler(orchestration)
	router.GET("/payment-attempts/:id", authenticatedTestCustomer(customerID), handler.GetPaymentAttempt)

	success := httptest.NewRecorder()
	router.ServeHTTP(success, httptest.NewRequest(http.MethodGet, "/payment-attempts/"+allowedAttemptID, nil))
	if success.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s; want 200", success.Code, success.Body.String())
	}
	var successBody struct {
		PaymentAttempt PaymentAttemptView `json:"payment_attempt"`
	}
	if err := json.Unmarshal(success.Body.Bytes(), &successBody); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if successBody.PaymentAttempt.ID != allowedAttemptID ||
		successBody.PaymentAttempt.CheckoutURL == nil ||
		*successBody.PaymentAttempt.CheckoutURL != checkoutURL {
		t.Fatalf("unexpected get response: %#v", successBody)
	}

	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/payment-attempts/"+deniedAttemptID, nil))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("ownership denial status = %d body=%s; want 404", denied.Code, denied.Body.String())
	}
}

func TestResolvePaymentAttemptHTTPContractAndOwnershipDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	customerID := uuid.NewString()
	bookingID := uuid.NewString()
	attemptID := uuid.NewString()
	reference := "pa-" + strings.Repeat("a", 60)
	expiresAt := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	orchestration := &stubPaymentOrchestration{
		getPaymentByReference: func(_ context.Context, customer, receivedReference string) (PaymentAttemptView, error) {
			if customer != customerID || receivedReference != reference {
				return PaymentAttemptView{}, ErrPaymentAccessDenied
			}
			return PaymentAttemptView{
				ID:        attemptID,
				BookingID: bookingID,
				State:     AttemptStatePending,
				ExpiresAt: expiresAt,
			}, nil
		},
	}
	router := gin.New()
	handler := NewHandler(orchestration)
	router.GET("/payment-attempts/resolve/:reference", authenticatedTestCustomer(customerID), handler.ResolvePaymentAttempt)

	success := httptest.NewRecorder()
	router.ServeHTTP(success, httptest.NewRequest(http.MethodGet, "/payment-attempts/resolve/"+reference, nil))
	if success.Code != http.StatusOK {
		t.Fatalf("resolve status = %d body=%s; want 200", success.Code, success.Body.String())
	}
	var body struct {
		PaymentAttempt PaymentAttemptView `json:"payment_attempt"`
		StatusURL      string             `json:"status_url"`
	}
	if err := json.Unmarshal(success.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if body.PaymentAttempt.ID != attemptID || body.PaymentAttempt.BookingID != bookingID ||
		body.StatusURL != "/payment-attempts/"+attemptID {
		t.Fatalf("unexpected resolve response: %#v", body)
	}

	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/payment-attempts/resolve/not-owned", nil))
	if denied.Code != http.StatusNotFound {
		t.Fatalf("resolve denial status = %d body=%s; want 404", denied.Code, denied.Body.String())
	}
}

func TestPaymentHTTPErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid", err: ErrInvalidPaymentMethod, want: http.StatusBadRequest},
		{name: "conflict", err: ErrIdempotencyConflict, want: http.StatusConflict},
		{name: "disabled", err: ErrPaymentCapabilityDisabled, want: http.StatusServiceUnavailable},
		{name: "not found", err: ErrPaymentAccessDenied, want: http.StatusNotFound},
		{name: "audit unavailable", err: ErrPaymentAuditUnavailable, want: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ginContext, _ := gin.CreateTestContext(recorder)
			respondPaymentError(ginContext, tc.err)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d; want %d", recorder.Code, tc.want)
			}
		})
	}
}

type stubPaymentOrchestration struct {
	createPayment         func(context.Context, string, string, string, CreateAttemptRequest) (CreatePaymentResult, error)
	getPayment            func(context.Context, string, string) (PaymentAttemptView, error)
	getPaymentByReference func(context.Context, string, string) (PaymentAttemptView, error)
}

func (s *stubPaymentOrchestration) CreatePayment(ctx context.Context, customerID, bookingID, idempotencyKey string, req CreateAttemptRequest) (CreatePaymentResult, error) {
	return s.createPayment(ctx, customerID, bookingID, idempotencyKey, req)
}

func (s *stubPaymentOrchestration) GetPaymentAttempt(ctx context.Context, customerID, attemptID string) (PaymentAttemptView, error) {
	return s.getPayment(ctx, customerID, attemptID)
}

func (s *stubPaymentOrchestration) GetPaymentAttemptByReference(ctx context.Context, customerID, localReference string) (PaymentAttemptView, error) {
	return s.getPaymentByReference(ctx, customerID, localReference)
}

func authenticatedTestCustomer(customerID string) gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Set("auth_user_id", customerID)
		context.Next()
	}
}
