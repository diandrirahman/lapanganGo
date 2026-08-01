package paymentwebhooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"lapangango-api/internal/payments"

	"github.com/gin-gonic/gin"
)

type memoryRepository struct {
	accepted     []AcceptParams
	unsupported  []UnsupportedParams
	authFailures []AuthFailureParams
	attempt      *AttemptContext
	err          error
}

func (r *memoryRepository) FindAttemptContext(context.Context, payments.WebhookEvent) (*AttemptContext, error) {
	return r.attempt, r.err
}
func (r *memoryRepository) Accept(_ context.Context, p AcceptParams) (Acceptance, error) {
	if r.err != nil {
		return Acceptance{}, r.err
	}
	r.accepted = append(r.accepted, p)
	return Acceptance{New: true}, nil
}
func (r *memoryRepository) RecordUnsupported(_ context.Context, p UnsupportedParams) error {
	if r.err != nil {
		return r.err
	}
	r.unsupported = append(r.unsupported, p)
	return nil
}
func (r *memoryRepository) RecordAuthFailure(_ context.Context, p AuthFailureParams) error {
	r.authFailures = append(r.authFailures, p)
	return r.err
}

func TestIngressAcceptsNormalizedWebhookWithoutJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, err := payments.NewXenditTestWebhookVerifier("test-callback-token")
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{}
	service, err := NewService(verifier, repo, func() time.Time { return time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service)
	handler.correlationID = func() string { return "webhook:test-correlation" }
	router := gin.New()
	handler.RegisterRoutes(router)
	body := `{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:00:00Z","data":{"payment_id":"pay_fixture_0001","payment_request_id":"pr_fixture_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`
	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Callback-Token", "test-callback-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"category":"accepted"`) || !strings.Contains(response.Body.String(), "webhook:test-correlation") {
		t.Fatalf("response is not generic accepted response: %s", response.Body.String())
	}
	if len(repo.accepted) != 1 || repo.accepted[0].Event.EventKey != "XENDIT|payment.capture|pay_fixture_0001" {
		t.Fatalf("accepted = %#v", repo.accepted)
	}
	if strings.Contains(response.Body.String(), "pay_fixture") || strings.Contains(response.Body.String(), "test-callback-token") {
		t.Fatal("response leaked provider identity or token")
	}
}

func TestIngressRejectsInvalidAuthAndDoesNotPersist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	repo := &memoryRepository{}
	service, _ := NewService(verifier, repo, nil)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader(`{"event":"payment.capture"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || len(repo.accepted) != 0 || len(repo.authFailures) != 1 {
		t.Fatalf("status=%d accepted=%d authFailures=%d", response.Code, len(repo.accepted), len(repo.authFailures))
	}
	if strings.Contains(response.Body.String(), "token") {
		t.Fatalf("auth response leaked token detail: %s", response.Body.String())
	}
}

func TestIngressRejectsWrongRouteAsQuarantine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	repo := &memoryRepository{}
	service, _ := NewService(verifier, repo, nil)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	body := `{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:00:00Z","data":{"payment_id":"pay_fixture_0002","status":"PENDING","amount":125000,"currency":"IDR"}}`
	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/refund", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Callback-Token", "test-callback-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(repo.accepted) != 1 {
		t.Fatalf("status=%d accepted=%#v", response.Code, repo.accepted)
	}
	if repo.accepted[0].Event.VerificationState != payments.WebhookVerificationQuarantined || repo.accepted[0].Event.ReasonCode != string(payments.AdapterErrorInvalidRequest) {
		t.Fatalf("wrong route event was not quarantined: %#v", repo.accepted[0].Event)
	}
}

func TestIngressUsesReadOnlyAttemptContextForMismatchQuarantine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	repo := &memoryRepository{attempt: &AttemptContext{ID: "00000000-0000-0000-0000-000000000001", AmountRupiah: 125000, Currency: payments.CurrencyIDR, PaymentID: "pay_fixture_amount_0001", PaymentRequestID: "pr_fixture_amount_0001"}}
	service, _ := NewService(verifier, repo, nil)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	body := `{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:00:00Z","data":{"payment_id":"pay_fixture_amount_0001","payment_request_id":"pr_fixture_amount_0001","status":"PENDING","amount":125001,"currency":"IDR"}}`
	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Callback-Token", "test-callback-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(repo.accepted) != 1 {
		t.Fatalf("status=%d accepted=%#v", response.Code, repo.accepted)
	}
	got := repo.accepted[0]
	if got.PaymentAttemptID == nil || *got.PaymentAttemptID != repo.attempt.ID || got.Event.VerificationState != payments.WebhookVerificationQuarantined || got.Event.ReasonCode != string(payments.AdapterErrorAmountMismatch) {
		t.Fatalf("attempt context was not applied safely: %#v", got)
	}
}

func TestIngressUnsupportedSchemaCreatesSanitizedDurableReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	repo := &memoryRepository{}
	service, _ := NewService(verifier, repo, nil)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	body := `{"event":"payment.capture","version":"2099-01-01","created":"2026-01-15T10:00:00Z","data":{"payment_id":"pay_fixture_0003","status":"PENDING","amount":125000,"currency":"IDR"}}`
	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Callback-Token", "test-callback-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(repo.accepted) != 0 || len(repo.unsupported) != 1 {
		t.Fatalf("status=%d accepted=%d unsupported=%d", response.Code, len(repo.accepted), len(repo.unsupported))
	}
	if repo.unsupported[0].RawBodyHash == "" || strings.Contains(response.Body.String(), "2099") {
		t.Fatalf("unsupported response leaked provider detail: %#v %s", repo.unsupported[0], response.Body.String())
	}
}

func TestIngressRejectsContentAndBoundedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	service, _ := NewService(verifier, &memoryRepository{}, nil)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)
	wrongType := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader("{}"))
	wrongType.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, wrongType)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type status=%d", response.Code)
	}
	tooLarge := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader(strings.Repeat("x", payments.XenditWebhookMaxBodyBytes+1)))
	tooLarge.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, tooLarge)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body limit status=%d", response.Code)
	}
}

type blockingRequestBody struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingRequestBody() *blockingRequestBody {
	return &blockingRequestBody{closed: make(chan struct{})}
}

func (b *blockingRequestBody) Read([]byte) (int, error) {
	<-b.closed
	return 0, context.Canceled
}

func (b *blockingRequestBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func TestIngressDeadlineCoversSlowBodyRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	service, _ := NewService(verifier, &memoryRepository{}, nil)
	handler := NewHandler(service)
	handler.deadline = 20 * time.Millisecond
	handler.correlationID = func() string { return "webhook:deadline-test" }
	router := gin.New()
	handler.RegisterRoutes(router)

	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", nil)
	request.Body = newBlockingRequestBody()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Callback-Token", "test-callback-token")
	response := httptest.NewRecorder()
	started := time.Now()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("slow body status=%d body=%s", response.Code, response.Body.String())
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("slow body deadline took %s", elapsed)
	}
	if !strings.Contains(response.Body.String(), `"category":"temporarily_unavailable"`) || strings.Contains(response.Body.String(), "token") {
		t.Fatalf("deadline response is not generic: %s", response.Body.String())
	}
}

func TestIngressRejectsJSONStructureLimitsAfterAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	service, _ := NewService(verifier, &memoryRepository{}, nil)
	router := gin.New()
	NewHandler(service).RegisterRoutes(router)

	deepBody := strings.Repeat(`{"x":`, 17) + `1` + strings.Repeat(`}`, 17)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader(deepBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Callback-Token", "test-callback-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("deep JSON status=%d", response.Code)
	}

	members := make([]string, 129)
	for index := range members {
		members[index] = `"key` + strconv.Itoa(index) + `":1`
	}
	request = httptest.NewRequest(http.MethodPost, "/webhooks/xendit/payment", strings.NewReader(`{`+strings.Join(members, ",")+`}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Callback-Token", "test-callback-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("member limit status=%d", response.Code)
	}
}

func TestRateLimiterUsesBurstCapacity(t *testing.T) {
	limiter := NewRateLimiter(120, 2, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	if !limiter.allow("payment:127.0.0.1") || !limiter.allow("payment:127.0.0.1") || limiter.allow("payment:127.0.0.1") {
		t.Fatal("burst capacity was not enforced")
	}
	now = now.Add(time.Second)
	if !limiter.allow("payment:127.0.0.1") {
		t.Fatal("per-minute refill was not applied")
	}
}

func TestRateLimiterIsGlobalPerRouteAndRoutesAreIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, _ := payments.NewXenditTestWebhookVerifier("test-callback-token")
	service, _ := NewService(verifier, &memoryRepository{}, nil)
	handler := NewHandler(service)
	handler.correlationID = func() string { return "webhook:rate-limit-test" }
	handler.limiter = NewRateLimiter(120, 2, time.Minute)
	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	handler.limiter.now = func() time.Time { return fixedNow }
	router := gin.New()
	handler.RegisterRoutes(router)

	request := func(path, remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.RemoteAddr = remoteAddr
		req.Header.Set("Content-Type", "text/plain")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response.Code
	}

	if status := request("/webhooks/xendit/payment", "192.0.2.1:1001"); status != http.StatusUnsupportedMediaType {
		t.Fatalf("first request status=%d", status)
	}
	if status := request("/webhooks/xendit/payment", "192.0.2.2:1002"); status != http.StatusUnsupportedMediaType {
		t.Fatalf("second request status=%d", status)
	}
	if status := request("/webhooks/xendit/payment", "192.0.2.3:1003"); status != http.StatusTooManyRequests {
		t.Fatalf("cross-IP route limit status=%d", status)
	}
	if status := request("/webhooks/xendit/refund", "192.0.2.3:1003"); status != http.StatusUnsupportedMediaType {
		t.Fatalf("independent route status=%d", status)
	}
}
