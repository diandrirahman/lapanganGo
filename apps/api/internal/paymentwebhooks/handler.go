package paymentwebhooks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service       *Service
	correlationID func() string
	deadline      time.Duration
	limiter       *RateLimiter
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service:       service,
		correlationID: newCorrelationID,
		deadline:      ingressDeadline,
		limiter:       NewRateLimiter(120, 30, time.Minute),
	}
}

func (h *Handler) RegisterRoutes(router *gin.Engine) {
	group := router.Group("/webhooks/xendit")
	group.Use(correlationMiddleware(h.correlationID), h.limiter.Handle())
	group.POST("/payment-session", h.receive(RoutePaymentSession))
	group.POST("/payment", h.receive(RoutePayment))
	group.POST("/refund", h.receive(RouteRefund))
}

func (h *Handler) receive(family RouteFamily) gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.GetString("payment_webhook_correlation_id")
		ctx, cancel := context.WithTimeout(c.Request.Context(), h.deadline)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		receivedAt := time.Now().UTC()
		contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || contentType != "application/json" {
			respond(c, http.StatusUnsupportedMediaType, UnsupportedMediaCategory, correlationID)
			return
		}
		body, err := readBoundedBody(c)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				respond(c, http.StatusServiceUnavailable, UnavailableCategory, correlationID)
				return
			}
			respond(c, http.StatusRequestEntityTooLarge, PayloadTooLargeCategory, correlationID)
			return
		}
		values := c.Request.Header.Values("X-Callback-Token")
		headers := map[string]string{}
		if len(values) == 1 {
			headers["x-callback-token"] = values[0]
		}
		result, _ := h.service.Receive(ctx, ReceiveRequest{RouteFamily: family, RawBody: body, Headers: headers, ReceivedAt: receivedAt, CorrelationID: correlationID})
		respond(c, result.Status, result.Category, correlationID)
	}
}

func readBoundedBody(c *gin.Context) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(256*1024))
	defer c.Request.Body.Close()
	type readResult struct {
		body []byte
		err  error
	}
	result := make(chan readResult, 1)
	go func() {
		body, err := ioReadAll(c.Request.Body)
		result <- readResult{body: body, err: err}
	}()
	select {
	case <-c.Request.Context().Done():
		_ = c.Request.Body.Close()
		return nil, c.Request.Context().Err()
	case out := <-result:
		if out.err != nil {
			return nil, out.err
		}
		return out.body, nil
	}
}

// ioReadAll is a variable so handler tests can cover read failure without a
// network server. It never logs the received bytes.
var ioReadAll = func(body io.Reader) ([]byte, error) { return io.ReadAll(body) }

func respond(c *gin.Context, status int, category, correlationID string) {
	c.JSON(status, gin.H{"category": category, "correlation_id": correlationID})
}

func correlationMiddleware(newID func() string) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set("payment_webhook_correlation_id", newID()); c.Next() }
}

func newCorrelationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "webhook-correlation-unavailable"
	}
	return "webhook:" + hex.EncodeToString(b)
}
