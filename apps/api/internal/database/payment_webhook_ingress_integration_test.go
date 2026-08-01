package database_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/payments"
	"lapangango-api/internal/paymentwebhooks"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPaymentWebhookIngressRepository_DurableDuplicateAndConflict(t *testing.T) {
	adminDSN := checkOptIn(t)
	targetDSN, cleanup := createDisposableDB(t, adminDSN)
	defer cleanup()

	db, migration := setupMigrate(t, targetDSN)
	defer db.Close()
	defer migration.Close()
	migrateToVersion(t, migration, 29)
	assertMigrationVersion(t, migration, 29, false)
	t.Log("disposable migration evidence: 29|false")

	pool, err := pgxpool.New(context.Background(), targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	verifier, err := payments.NewXenditTestWebhookVerifier("test-callback-token")
	if err != nil {
		t.Fatal(err)
	}
	repository := paymentwebhooks.NewPostgresRepository(pool, audit.NewPlatformRepository())
	service, err := paymentwebhooks.NewService(verifier, repository, nil)
	if err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)
	body := []byte(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:00:00Z","data":{"payment_id":"pay_ingress_capture_0001","payment_request_id":"pr_ingress_capture_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`)
	request := paymentwebhooks.ReceiveRequest{RouteFamily: paymentwebhooks.RoutePayment, RawBody: body, Headers: map[string]string{"x-callback-token": "test-callback-token"}, ReceivedAt: receivedAt, CorrelationID: "webhook:integration-0001"}
	result, err := service.Receive(context.Background(), request)
	if err != nil || !result.Accepted || result.Status != 200 {
		event, parseErr := verifier.ParseWebhook(context.Background(), payments.ParseWebhookRequest{RawBody: body, ObservedAt: receivedAt, MaxBodyBytes: payments.XenditWebhookMaxBodyBytes})
		if parseErr != nil {
			t.Fatalf("first receive: result=%+v err=%v; diagnostic parse=%v", result, err, parseErr)
		}
		if _, lookupErr := repository.FindAttemptContext(context.Background(), event); lookupErr != nil {
			t.Fatalf("first receive: result=%+v err=%v; diagnostic lookup=%v", result, err, lookupErr)
		}
		_, acceptErr := repository.Accept(context.Background(), paymentwebhooks.AcceptParams{Event: event, AuthContract: payments.XenditWebhookAuthContractVersion, CorrelationID: request.CorrelationID, RouteFamily: request.RouteFamily, ReceivedAt: request.ReceivedAt})
		t.Fatalf("first receive: result=%+v err=%v; diagnostic accept=%v", result, err, acceptErr)
	}
	result, err = service.Receive(context.Background(), request)
	if err != nil || !result.Accepted {
		t.Fatalf("duplicate receive: result=%+v err=%v", result, err)
	}

	conflict := append([]byte(nil), body...)
	conflict = []byte(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:00:01Z","data":{"payment_id":"pay_ingress_capture_0001","payment_request_id":"pr_ingress_capture_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`)
	request.RawBody = conflict
	request.CorrelationID = "webhook:integration-0002"
	result, err = service.Receive(context.Background(), request)
	if err != nil || !result.Accepted {
		t.Fatalf("conflict receive: result=%+v err=%v", result, err)
	}

	var eventCount, auditCount int
	var verification, processing, storedHash string
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM payment_webhook_events`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("inbox count = %d; want 1", eventCount)
	}
	if err := pool.QueryRow(context.Background(), `SELECT verification_state, processing_state, raw_body_hash FROM payment_webhook_events WHERE provider_event_key = 'XENDIT|payment.capture|pay_ingress_capture_0001'`).Scan(&verification, &processing, &storedHash); err != nil {
		t.Fatal(err)
	}
	if verification != "QUARANTINED" || processing != "TERMINAL" {
		t.Fatalf("conflict lifecycle = %s/%s", verification, processing)
	}
	conflictDigest := sha256.Sum256(conflict)
	if storedHash == hex.EncodeToString(conflictDigest[:]) {
		t.Fatal("conflict overwrote immutable raw hash")
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM platform_audit_logs WHERE entity_type = 'PAYMENT_WEBHOOK'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 4 {
		t.Fatalf("webhook audit count = %d; want at least 4", auditCount)
	}
	var conflictResult, conflictReason string
	if err := pool.QueryRow(context.Background(), `
		SELECT metadata->>'result', metadata->>'reason'
		FROM platform_audit_logs
		WHERE action = 'webhook_conflict' AND correlation_id = 'webhook:integration-0002'
	`).Scan(&conflictResult, &conflictReason); err != nil {
		t.Fatal(err)
	}
	if conflictResult != "CONFLICT" || conflictReason != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("conflict audit = %s/%s", conflictResult, conflictReason)
	}

	quarantineCases := []struct {
		name           string
		body           string
		route          paymentwebhooks.RouteFamily
		eventKey       string
		receivedAt     time.Time
		expectedReason string
	}{
		{
			name:  "currency mismatch",
			body:  `{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:07:00Z","data":{"payment_id":"pay_ingress_currency_0001","payment_request_id":"pr_ingress_currency_0001","status":"SUCCEEDED","amount":125000,"currency":"USD"}}`,
			route: paymentwebhooks.RoutePayment, eventKey: "XENDIT|payment.capture|pay_ingress_currency_0001",
			receivedAt: receivedAt, expectedReason: "CURRENCY_MISMATCH",
		},
		{
			name:  "future timestamp",
			body:  `{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T11:05:01Z","data":{"payment_id":"pay_ingress_future_0001","payment_request_id":"pr_ingress_future_0001","status":"SUCCEEDED","amount":125000,"currency":"IDR"}}`,
			route: paymentwebhooks.RoutePayment, eventKey: "XENDIT|payment.capture|pay_ingress_future_0001",
			receivedAt: receivedAt, expectedReason: "FUTURE_CREATED_SEMANTIC",
		},
		{
			name:  "wrong route",
			body:  `{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:08:00Z","data":{"payment_id":"pay_ingress_wrong_route_0001","payment_request_id":"pr_ingress_wrong_route_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`,
			route: paymentwebhooks.RouteRefund, eventKey: "XENDIT|payment.capture|pay_ingress_wrong_route_0001",
			receivedAt: receivedAt, expectedReason: "INVALID_REQUEST",
		},
	}
	for index, tc := range quarantineCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.Receive(context.Background(), paymentwebhooks.ReceiveRequest{
				RouteFamily: tc.route, RawBody: []byte(tc.body), Headers: map[string]string{"x-callback-token": "test-callback-token"},
				ReceivedAt: tc.receivedAt, CorrelationID: fmt.Sprintf("webhook:quarantine-%04d", index+1),
			})
			if err != nil || !result.Accepted || result.Status != 200 {
				t.Fatalf("receive: result=%+v err=%v", result, err)
			}
			var gotVerification, gotProcessing, gotReason string
			var timestampValid bool
			if err := pool.QueryRow(context.Background(), `
				SELECT verification_state, processing_state, redacted_payload->>'reason_code',
				       processed_at >= created_at AND processed_at >= received_at
				FROM payment_webhook_events WHERE provider_event_key = $1
			`, tc.eventKey).Scan(&gotVerification, &gotProcessing, &gotReason, &timestampValid); err != nil {
				t.Fatal(err)
			}
			if gotVerification != "QUARANTINED" || gotProcessing != "TERMINAL" || gotReason != tc.expectedReason || !timestampValid {
				t.Fatalf("quarantine = %s/%s reason=%s timestampValid=%t", gotVerification, gotProcessing, gotReason, timestampValid)
			}
		})
	}

	for index, initialState := range []string{"PROCESSING", "RETRYABLE"} {
		paymentID := fmt.Sprintf("pay_ingress_lifecycle_%04d", index+1)
		original := []byte(fmt.Sprintf(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:20:0%dZ","data":{"payment_id":"%s","status":"PENDING","amount":125000,"currency":"IDR"}}`, index, paymentID))
		lifecycleRequest := paymentwebhooks.ReceiveRequest{
			RouteFamily: paymentwebhooks.RoutePayment, RawBody: original, Headers: map[string]string{"x-callback-token": "test-callback-token"},
			ReceivedAt: receivedAt, CorrelationID: fmt.Sprintf("webhook:lifecycle-new-%04d", index+1),
		}
		if result, err := service.Receive(context.Background(), lifecycleRequest); err != nil || !result.Accepted {
			t.Fatalf("seed %s: result=%+v err=%v", initialState, result, err)
		}
		key := "XENDIT|payment.capture|" + paymentID
		if _, err := pool.Exec(context.Background(), `UPDATE payment_webhook_events SET processing_state=$1, updated_at=transaction_timestamp() WHERE provider_event_key=$2`, initialState, key); err != nil {
			t.Fatalf("set %s: %v", initialState, err)
		}
		var originalHash, originalPayload string
		if err := pool.QueryRow(context.Background(), `SELECT raw_body_hash, redacted_payload::text FROM payment_webhook_events WHERE provider_event_key=$1`, key).Scan(&originalHash, &originalPayload); err != nil {
			t.Fatal(err)
		}
		lifecycleRequest.RawBody = []byte(fmt.Sprintf(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:21:0%dZ","data":{"payment_id":"%s","status":"PENDING","amount":125000,"currency":"IDR"}}`, index, paymentID))
		lifecycleRequest.CorrelationID = fmt.Sprintf("webhook:lifecycle-conflict-%04d", index+1)
		if result, err := service.Receive(context.Background(), lifecycleRequest); err != nil || !result.Accepted {
			t.Fatalf("conflict %s: result=%+v err=%v", initialState, result, err)
		}
		var gotVerification, gotProcessing, gotHash, gotPayload string
		if err := pool.QueryRow(context.Background(), `SELECT verification_state, processing_state, raw_body_hash, redacted_payload::text FROM payment_webhook_events WHERE provider_event_key=$1`, key).Scan(&gotVerification, &gotProcessing, &gotHash, &gotPayload); err != nil {
			t.Fatal(err)
		}
		if gotVerification != "QUARANTINED" || gotProcessing != "TERMINAL" || gotHash != originalHash || gotPayload != originalPayload {
			t.Fatalf("conflict %s lifecycle=%s/%s immutable=%t/%t", initialState, gotVerification, gotProcessing, gotHash == originalHash, gotPayload == originalPayload)
		}
	}

	terminalPaymentID := "pay_ingress_terminal_0001"
	terminalBody := []byte(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:30:00Z","data":{"payment_id":"pay_ingress_terminal_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`)
	terminalRequest := paymentwebhooks.ReceiveRequest{RouteFamily: paymentwebhooks.RoutePayment, RawBody: terminalBody, Headers: map[string]string{"x-callback-token": "test-callback-token"}, ReceivedAt: receivedAt, CorrelationID: "webhook:terminal-new-0001"}
	if result, err := service.Receive(context.Background(), terminalRequest); err != nil || !result.Accepted {
		t.Fatalf("terminal seed: result=%+v err=%v", result, err)
	}
	terminalKey := "XENDIT|payment.capture|" + terminalPaymentID
	if _, err := pool.Exec(context.Background(), `UPDATE payment_webhook_events SET processing_state='PROCESSED', processed_at=transaction_timestamp(), updated_at=transaction_timestamp() WHERE provider_event_key=$1`, terminalKey); err != nil {
		t.Fatal(err)
	}
	var terminalBefore string
	if err := pool.QueryRow(context.Background(), `SELECT row_to_json(e)::text FROM payment_webhook_events e WHERE provider_event_key=$1`, terminalKey).Scan(&terminalBefore); err != nil {
		t.Fatal(err)
	}
	terminalRequest.RawBody = []byte(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:30:01Z","data":{"payment_id":"pay_ingress_terminal_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`)
	terminalRequest.CorrelationID = "webhook:terminal-conflict-0001"
	if result, err := service.Receive(context.Background(), terminalRequest); err != nil || !result.Accepted {
		t.Fatalf("terminal conflict: result=%+v err=%v", result, err)
	}
	var terminalAfter string
	if err := pool.QueryRow(context.Background(), `SELECT row_to_json(e)::text FROM payment_webhook_events e WHERE provider_event_key=$1`, terminalKey).Scan(&terminalAfter); err != nil {
		t.Fatal(err)
	}
	if terminalAfter != terminalBefore {
		t.Fatal("conflicting replay mutated an already-terminal inbox row")
	}

	unsupportedBody := []byte(`{"event":"payment.capture","version":"2099-01-01","created":"2026-01-15T10:40:00Z","data":{"payment_id":"pay_ingress_unsupported_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`)
	unsupportedRequest := paymentwebhooks.ReceiveRequest{RouteFamily: paymentwebhooks.RoutePayment, RawBody: unsupportedBody, Headers: map[string]string{"x-callback-token": "test-callback-token"}, ReceivedAt: receivedAt, CorrelationID: "webhook:unsupported-0001"}
	if result, err := service.Receive(context.Background(), unsupportedRequest); err != nil || !result.Accepted || result.Status != 200 {
		t.Fatalf("unsupported: result=%+v err=%v", result, err)
	}
	var unsupportedInbox, unsupportedAudit int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM payment_webhook_events WHERE correlation_id='webhook:unsupported-0001'`).Scan(&unsupportedInbox); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM platform_audit_logs WHERE correlation_id='webhook:unsupported-0001' AND metadata->>'result'='UNSUPPORTED' AND metadata->>'reason'='INVALID_REQUEST'`).Scan(&unsupportedAudit); err != nil {
		t.Fatal(err)
	}
	if unsupportedInbox != 0 || unsupportedAudit != 1 {
		t.Fatalf("unsupported durability inbox=%d audit=%d", unsupportedInbox, unsupportedAudit)
	}

	sensitiveBody := []byte(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:45:00Z","data":{"payment_id":"pay_ingress_sensitive_0001","payment_request_id":"pr_ingress_sensitive_0001","status":"PENDING","amount":125000,"currency":"IDR","pan":"<redacted>","cvv":"<redacted>","callback_token":"<redacted>","customer":{"email":"<redacted>"}}}`)
	sensitiveRequest := paymentwebhooks.ReceiveRequest{RouteFamily: paymentwebhooks.RoutePayment, RawBody: sensitiveBody, Headers: map[string]string{"x-callback-token": "test-callback-token"}, ReceivedAt: receivedAt, CorrelationID: "webhook:sensitive-0001"}
	if result, err := service.Receive(context.Background(), sensitiveRequest); err != nil || !result.Accepted {
		t.Fatalf("sensitive: result=%+v err=%v", result, err)
	}
	var sensitivePayload string
	if err := pool.QueryRow(context.Background(), `SELECT redacted_payload::text FROM payment_webhook_events WHERE correlation_id='webhook:sensitive-0001'`).Scan(&sensitivePayload); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"pan", "cvv", "callback_token", "customer", "email", "<redacted>"} {
		if strings.Contains(strings.ToLower(sensitivePayload), forbidden) {
			t.Fatalf("sensitive payload retained %q: %s", forbidden, sensitivePayload)
		}
	}

	pool.Close()
	databaseFailureBody := []byte(`{"event":"payment.capture","version":"2024-11-11","created":"2026-01-15T10:50:00Z","data":{"payment_id":"pay_ingress_db_failure_0001","status":"PENDING","amount":125000,"currency":"IDR"}}`)
	databaseFailureRequest := paymentwebhooks.ReceiveRequest{RouteFamily: paymentwebhooks.RoutePayment, RawBody: databaseFailureBody, Headers: map[string]string{"x-callback-token": "test-callback-token"}, ReceivedAt: receivedAt, CorrelationID: "webhook:db-failure-0001"}
	result, err = service.Receive(context.Background(), databaseFailureRequest)
	if !errors.Is(err, paymentwebhooks.ErrDurabilityUnavailable) || result.Accepted || result.Status != 503 {
		t.Fatalf("database failure: result=%+v err=%v", result, err)
	}
}
