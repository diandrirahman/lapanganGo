package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"lapangango-api/internal/httputil"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	orchestrator PaymentOrchestration
}

type PaymentOrchestration interface {
	CreatePayment(ctx context.Context, customerID, bookingID, idempotencyKey string, req CreateAttemptRequest) (CreatePaymentResult, error)
	GetPaymentAttempt(ctx context.Context, customerID, attemptID string) (PaymentAttemptView, error)
	GetPaymentAttemptByReference(ctx context.Context, customerID, localReference string) (PaymentAttemptView, error)
}

func NewHandler(orchestrator PaymentOrchestration) *Handler {
	return &Handler{orchestrator: orchestrator}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware, requireActiveUser, customerRoleMiddleware gin.HandlerFunc) {
	bookingGroup := router.Group("/bookings", authMiddleware, requireActiveUser, customerRoleMiddleware)
	bookingGroup.POST("/:id/payment-attempts", h.CreatePaymentAttempt)

	attemptGroup := router.Group("/payment-attempts", authMiddleware, requireActiveUser, customerRoleMiddleware)
	attemptGroup.GET("/resolve/:reference", h.ResolvePaymentAttempt)
	attemptGroup.GET("/:id", h.GetPaymentAttempt)
}

type createPaymentAttemptHTTPRequest struct {
	RequestedMethod string `json:"requested_method"`
}

const maxCreatePaymentRequestBodyBytes int64 = 4096

func (h *Handler) CreatePaymentAttempt(c *gin.Context) {
	customerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}
	bookingID := c.Param("id")
	if !httputil.IsUUID(bookingID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid booking ID format"})
		return
	}
	idempotencyKey, err := singleIdempotencyKeyHeader(c.Request.Header)
	if err != nil {
		respondPaymentError(c, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreatePaymentRequestBodyBytes)
	body, err := decodeCreatePaymentAttemptHTTPRequest(c.Request.Body)
	if err != nil {
		respondPaymentJSONError(c, err)
		return
	}

	result, err := h.orchestrator.CreatePayment(c.Request.Context(), customerID, bookingID, idempotencyKey, CreateAttemptRequest{
		RequestedMethod: RequestedMethod(body.RequestedMethod),
	})
	if err != nil {
		respondPaymentError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"payment_attempt": paymentAttemptResponse(result.Attempt),
		"status_url":      "/payment-attempts/" + result.Attempt.ID,
		"replayed":        result.Replay,
	})
}

func singleIdempotencyKeyHeader(header http.Header) (string, error) {
	values := header.Values("Idempotency-Key")
	if len(values) != 1 || !validIdempotencyKey(values[0]) {
		return "", ErrInvalidIdempotencyKey
	}
	return values[0], nil
}

func decodeCreatePaymentAttemptHTTPRequest(body io.Reader) (createPaymentAttemptHTTPRequest, error) {
	decoder := json.NewDecoder(body)
	firstToken, err := decoder.Token()
	if err != nil {
		return createPaymentAttemptHTTPRequest{}, err
	}
	objectStart, ok := firstToken.(json.Delim)
	if !ok || objectStart != '{' {
		return createPaymentAttemptHTTPRequest{}, errors.New("payment request must be a JSON object")
	}

	var request createPaymentAttemptHTTPRequest
	requestedMethodSeen := false
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return createPaymentAttemptHTTPRequest{}, err
		}
		key, ok := keyToken.(string)
		if !ok || key != "requested_method" || requestedMethodSeen {
			return createPaymentAttemptHTTPRequest{}, errors.New("payment request contains an unknown or duplicate field")
		}
		requestedMethodSeen = true
		if err := decoder.Decode(&request.RequestedMethod); err != nil {
			return createPaymentAttemptHTTPRequest{}, err
		}
	}

	lastToken, err := decoder.Token()
	if err != nil {
		return createPaymentAttemptHTTPRequest{}, err
	}
	objectEnd, ok := lastToken.(json.Delim)
	if !ok || objectEnd != '}' {
		return createPaymentAttemptHTTPRequest{}, errors.New("payment request JSON object is incomplete")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return createPaymentAttemptHTTPRequest{}, errors.New("payment request contains trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return createPaymentAttemptHTTPRequest{}, err
	}
	return request, nil
}

func respondPaymentJSONError(c *gin.Context, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": "Payment request payload is too large"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request payload"})
}

func (h *Handler) GetPaymentAttempt(c *gin.Context) {
	customerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}
	attemptID := c.Param("id")
	if !httputil.IsUUID(attemptID) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid payment attempt ID format"})
		return
	}
	view, err := h.orchestrator.GetPaymentAttempt(c.Request.Context(), customerID, attemptID)
	if err != nil {
		respondPaymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"payment_attempt": view})
}

func (h *Handler) ResolvePaymentAttempt(c *gin.Context) {
	customerID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}
	view, err := h.orchestrator.GetPaymentAttemptByReference(
		c.Request.Context(),
		customerID,
		c.Param("reference"),
	)
	if err != nil {
		respondPaymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"payment_attempt": view,
		"status_url":      "/payment-attempts/" + view.ID,
	})
}

func paymentAttemptResponse(attempt PaymentAttempt) gin.H {
	return gin.H{
		"id":         attempt.ID,
		"booking_id": attempt.BookingID,
		"state":      attempt.State,
		"expires_at": attempt.ExpiresAt,
	}
}

func respondPaymentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidPaymentMethod), errors.Is(err, ErrInvalidIdempotencyKey), errors.Is(err, ErrInvalidCreateAttempt):
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid payment request"})
	case errors.Is(err, ErrIdempotencyConflict), errors.Is(err, ErrStateConflict), errors.Is(err, ErrBookingNotPayable), errors.Is(err, ErrSnapshotNotFound):
		c.JSON(http.StatusConflict, gin.H{"message": "Payment request cannot be created for this booking"})
	case errors.Is(err, ErrPaymentCapabilityDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Sandbox payment is currently disabled"})
	case errors.Is(err, ErrBookingNotFound), errors.Is(err, ErrPaymentAccessDenied), errors.Is(err, ErrAttemptNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "Payment attempt not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Internal server error"})
	}
}
