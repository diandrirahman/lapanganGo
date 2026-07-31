package payments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"lapangango-api/internal/audit"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

func TestPaymentDisposableSetupRegistersCleanupBeforeFallibleInitialization(t *testing.T) {
	var events []string
	var registeredCleanup func()
	setupFailure := errors.New("forced setup failure")

	err := initializePaymentDisposableAfterCreate(
		func(cleanup func()) {
			events = append(events, "register-cleanup")
			registeredCleanup = cleanup
		},
		func() {
			events = append(events, "cleanup")
		},
		func() error {
			events = append(events, "initialize")
			return setupFailure
		},
	)
	if !errors.Is(err, setupFailure) {
		t.Fatalf("setup error = %v; want forced setup failure", err)
	}
	if registeredCleanup == nil {
		t.Fatal("cleanup was not registered before initialization failure")
	}
	if strings.Join(events, ",") != "register-cleanup,initialize" {
		t.Fatalf("setup order = %v; want cleanup registration before initialization", events)
	}
	registeredCleanup()
	if strings.Join(events, ",") != "register-cleanup,initialize,cleanup" {
		t.Fatalf("cleanup execution = %v", events)
	}
}

func TestPaymentRepositoryDisposableEvidenceGate(t *testing.T) {
	if os.Getenv("REQUIRE_PAYMENT_REPOSITORY_DISPOSABLE") != "1" {
		t.Skip("repository disposable evidence gate is opt-in")
	}
	if os.Getenv("TEST_ROLLBACK_HARDENING_DISPOSABLE") != "1" {
		t.Fatal("repository disposable evidence required but TEST_ROLLBACK_HARDENING_DISPOSABLE is not 1")
	}
	if strings.TrimSpace(os.Getenv("ROLLBACK_HARDENING_TEST_DATABASE_URL")) == "" {
		t.Fatal("repository disposable evidence required but ROLLBACK_HARDENING_TEST_DATABASE_URL is empty")
	}
	t.Log("PAYMENT_REPOSITORY_DISPOSABLE_SUITE_ENABLED")
}

func TestRepositoryCreateReplayAndStateGuard(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)

	params := validCreateParams(bookingID, "payment:create:attempt-a")
	attempt, err := repo.CreateOrReplayAttempt(ctx, params)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if attempt.AttemptNo != 1 || attempt.State != AttemptStateCreated || attempt.AmountRupiah != 10000 {
		t.Fatalf("unexpected first attempt: %#v", attempt)
	}

	replay, err := repo.CreateOrReplayAttempt(ctx, params)
	if err != nil {
		t.Fatalf("replay attempt: %v", err)
	}
	if replay.ID != attempt.ID {
		t.Fatalf("replay created another attempt: first=%s replay=%s", attempt.ID, replay.ID)
	}

	expiringBookingID := seedRepositoryBooking(t, ctx, pool, true)
	expiringParams := validCreateParams(expiringBookingID, "payment:create:expired-replay")
	expiringParams.ExpiresAt = time.Now().UTC().Add(2 * time.Second)
	expiringAttempt, err := repo.CreateOrReplayAttempt(ctx, expiringParams)
	if err != nil {
		t.Fatalf("create expiring attempt: %v", err)
	}
	time.Sleep(time.Until(expiringParams.ExpiresAt) + 50*time.Millisecond)
	expiredReplay, err := repo.CreateOrReplayAttempt(ctx, expiringParams)
	if err != nil {
		t.Fatalf("replay expired attempt: %v", err)
	}
	if expiredReplay.ID != expiringAttempt.ID {
		t.Fatalf("expired replay created another attempt: first=%s replay=%s", expiringAttempt.ID, expiredReplay.ID)
	}

	overlongBookingID := seedRepositoryBooking(t, ctx, pool, true)
	overlongParams := validCreateParams(overlongBookingID, "payment:create:overlong-expiry")
	overlongParams.ExpiresAt = time.Now().UTC().Add(3 * time.Hour)
	if _, err := repo.CreateOrReplayAttempt(ctx, overlongParams); !errors.Is(err, ErrInvalidCreateAttempt) {
		t.Fatalf("attempt beyond booking expiry error = %v; want ErrInvalidCreateAttempt", err)
	}

	nextNo, err := repo.GetNextAttemptNumber(ctx, bookingID)
	if err != nil || nextNo != 2 {
		t.Fatalf("next attempt number = %d, %v; want 2, nil", nextNo, err)
	}
	attemptsByBooking, err := repo.GetAttemptsByBooking(ctx, bookingID)
	if err != nil || len(attemptsByBooking) != 1 || attemptsByBooking[0].ID != attempt.ID {
		t.Fatalf("attempts by booking = %#v, %v; want first attempt", attemptsByBooking, err)
	}
	if _, err := repo.CreateOrReplayAttempt(ctx, CreateAttemptParams{
		BookingID: bookingID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		RequestedMethod: RequestedMethodQRIS, IntegrationMode: IntegrationModePaymentLink,
		CaptureMethod: CaptureMethodAutomatic, LocalReference: params.LocalReference,
		RequestHash: strings.Repeat("f", 64), ExpiresAt: params.ExpiresAt,
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key/different hash error = %v; want ErrIdempotencyConflict", err)
	}

	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("created to pending: %v", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStatePending, AttemptStateCaptured); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("direct capture transition error = %v; want ErrInvalidTransition", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCreated, AttemptStateCancelled); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("stale CAS error = %v; want ErrStateConflict", err)
	}

	capture := validCaptureParams(attempt.ID, "payment-test-1", time.Now().UTC().Add(-time.Second))
	providerRequestID := "payment-request-test-1"
	capture.ProviderPaymentReqID = &providerRequestID
	captured, err := repo.RecordCapture(ctx, capture)
	if err != nil {
		t.Fatalf("record capture: %v", err)
	}
	if captured.Duplicate || captured.LateCapture || captured.Attempt.State != AttemptStateCaptured {
		t.Fatalf("unexpected normal capture result: %#v", captured)
	}

	duplicate, err := repo.RecordCapture(ctx, capture)
	if err != nil {
		t.Fatalf("duplicate capture: %v", err)
	}
	if !duplicate.Duplicate || duplicate.Fact.ID != captured.Fact.ID {
		t.Fatalf("duplicate capture was not a no-op: %#v", duplicate)
	}

	webhookDuplicate := capture
	webhookDuplicate.Authority = "VERIFIED_WEBHOOK"
	webhookDuplicate.ObservedAt = capture.ObservedAt.Add(time.Second)
	webhookDuplicate.SourceReference = "webhook:event-payment-test-1"
	webhookDuplicate.PayloadHash = strings.Repeat("e", 64)
	duplicate, err = repo.RecordCapture(ctx, webhookDuplicate)
	if err != nil {
		t.Fatalf("cross-authority duplicate capture: %v", err)
	}
	if !duplicate.Duplicate || duplicate.Fact.ID != captured.Fact.ID {
		t.Fatalf("cross-authority capture was not an idempotent no-op: %#v", duplicate)
	}

	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCaptured, AttemptStateFailed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("captured downgrade error = %v; want ErrInvalidTransition", err)
	}

	var factCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, attempt.ID).Scan(&factCount); err != nil {
		t.Fatalf("count capture facts: %v", err)
	}
	if factCount != 1 {
		t.Fatalf("capture fact count = %d; want 1", factCount)
	}
}

func TestRepositoryConcurrentCreateAndCapture(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)

	paramsA := validCreateParams(bookingID, "payment:create:concurrent-a")
	paramsB := validCreateParams(bookingID, "payment:create:concurrent-b")
	results := make(chan PaymentAttempt, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, params := range []CreateAttemptParams{paramsA, paramsB} {
		wg.Add(1)
		go func(params CreateAttemptParams) {
			defer wg.Done()
			attempt, err := repo.CreateOrReplayAttempt(ctx, params)
			if err != nil {
				errs <- err
				return
			}
			results <- attempt
		}(params)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent create: %v", err)
	}
	var attempts []PaymentAttempt
	for attempt := range results {
		attempts = append(attempts, attempt)
	}
	if len(attempts) != 2 || attempts[0].AttemptNo == attempts[1].AttemptNo {
		t.Fatalf("concurrent create attempt numbers = %#v; want distinct", attempts)
	}

	if _, err := repo.TransitionState(ctx, attempts[0].ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("prepare concurrent capture: %v", err)
	}
	capture := validCaptureParams(attempts[0].ID, "concurrent-payment", time.Now().UTC().Add(-time.Second))
	resultsCapture := make(chan CaptureResult, 2)
	errsCapture := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repo.RecordCapture(ctx, capture)
			if err != nil {
				errsCapture <- err
				return
			}
			resultsCapture <- result
		}()
	}
	wg.Wait()
	close(resultsCapture)
	close(errsCapture)
	for err := range errsCapture {
		t.Fatalf("concurrent capture: %v", err)
	}
	var duplicateCount int
	for result := range resultsCapture {
		if result.Duplicate {
			duplicateCount++
		}
	}
	if duplicateCount != 1 {
		t.Fatalf("duplicate concurrent capture count = %d; want 1", duplicateCount)
	}

	if _, err := repo.TransitionState(ctx, attempts[1].ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("prepare second capture: %v", err)
	}
	secondCapture := validCaptureParams(attempts[1].ID, "second-booking-payment", time.Now().UTC().Add(-time.Second))
	if _, err := repo.RecordCapture(ctx, secondCapture); !errors.Is(err, ErrAlreadyCaptured) {
		t.Fatalf("second booking capture error = %v; want ErrAlreadyCaptured", err)
	}
	secondState, err := repo.GetAttemptByID(ctx, attempts[1].ID)
	if err != nil {
		t.Fatalf("get second attempt after capture conflict: %v", err)
	}
	if secondState.State != AttemptStatePending || secondState.CapturedAt != nil {
		t.Fatalf("capture conflict changed second attempt: %#v", secondState)
	}
}

func TestRepositoryInquiryIdentityBindingIsExactAndAtomic(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingID, "payment:create:inquiry-identity"))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	sessionID := "session-inquiry-identity"
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyCreateProviderResultTx(ctx, tx, ApplyCreateProviderResultParams{
		AttemptID: attempt.ID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		ProviderSessionID: sessionID, ProviderStatusCode: "PENDING", Status: PaymentStatusPending,
		AmountRupiah: 10000, Currency: CurrencyIDR,
	}); err != nil {
		t.Fatalf("apply create result: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	requestID := "payment-request-inquiry-identity"
	sessionPtr := &sessionID
	requestPtr := &requestID
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, bound, err := repo.ApplyInquiryIdentityTx(ctx, tx, ApplyInquiryIdentityParams{
		AttemptID: attempt.ID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		Scope: PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionPtr,
		ProviderPaymentReqID: requestPtr, ProviderStatusCode: "PENDING",
	})
	if err != nil || !bound || updated.ProviderPaymentReqID == nil || *updated.ProviderPaymentReqID != requestID {
		t.Fatalf("first inquiry bind = %#v, bound=%v, err=%v", updated, bound, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, bound, err = repo.ApplyInquiryIdentityTx(ctx, tx, ApplyInquiryIdentityParams{
		AttemptID: attempt.ID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		Scope: PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionPtr,
		ProviderPaymentReqID: requestPtr, ProviderStatusCode: "PENDING",
	})
	if err != nil || bound {
		t.Fatalf("identical inquiry replay = bound=%v, err=%v", bound, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	wrongSession := "session-wrong"
	wrongRequest := "payment-request-wrong"
	for _, params := range []ApplyInquiryIdentityParams{
		{AttemptID: attempt.ID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest, Scope: PaymentInquiryScopeCheckoutSession, ProviderSessionID: &wrongSession, ProviderPaymentReqID: requestPtr, ProviderStatusCode: "PENDING"},
		{AttemptID: attempt.ID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest, Scope: PaymentInquiryScopeCheckoutSession, ProviderSessionID: sessionPtr, ProviderPaymentReqID: &wrongRequest, ProviderStatusCode: "PENDING"},
	} {
		tx, err = pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := repo.ApplyInquiryIdentityTx(ctx, tx, params); !errors.Is(err, ErrCaptureConflict) {
			_ = tx.Rollback(ctx)
			t.Fatalf("mismatched identity error = %v; want ErrCaptureConflict", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}

	stored, err := repo.GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ProviderPaymentReqID == nil || *stored.ProviderPaymentReqID != requestID || stored.ProviderPaymentID != nil {
		t.Fatalf("mismatched identity mutated attempt: %#v", stored)
	}

	bookingBeforeRequest := seedRepositoryBooking(t, ctx, pool, true)
	attemptBeforeRequest, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingBeforeRequest, "payment:create:payment-before-request"))
	if err != nil {
		t.Fatal(err)
	}
	bindTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyCreateProviderResultTx(ctx, bindTx, ApplyCreateProviderResultParams{AttemptID: attemptBeforeRequest.ID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest, ProviderSessionID: "session-before-request", ProviderStatusCode: "PENDING", Status: PaymentStatusPending, AmountRupiah: 10000, Currency: CurrencyIDR}); err != nil {
		_ = bindTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := bindTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	bindTx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.ApplyInquiryIdentityTx(ctx, bindTx, ApplyInquiryIdentityParams{AttemptID: attemptBeforeRequest.ID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest, Scope: PaymentInquiryScopePayment, ProviderPaymentReqID: &requestID, ProviderPaymentID: &wrongRequest, ProviderStatusCode: "PENDING"}); !errors.Is(err, ErrCaptureConflict) {
		_ = bindTx.Rollback(ctx)
		t.Fatalf("payment before request error = %v; want ErrCaptureConflict", err)
	}
	if err := bindTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestApplyCreateProviderResultTerminalNoopRejectsIdentityMismatch(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingID, "payment:create:terminal-create-identity"))
	if err != nil {
		t.Fatal(err)
	}
	winnerSessionID := "session-terminal-create-winner-0001"
	apply := func(sessionID string) (PaymentAttempt, error) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return PaymentAttempt{}, err
		}
		defer tx.Rollback(ctx)
		result, err := repo.ApplyCreateProviderResultTx(ctx, tx, ApplyCreateProviderResultParams{
			AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment,
			ProviderSessionID: sessionID, ProviderStatusCode: "PENDING", Status: PaymentStatusPending,
			AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency,
		})
		if err != nil {
			return PaymentAttempt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return PaymentAttempt{}, err
		}
		return result, nil
	}
	if _, err := apply(winnerSessionID); err != nil {
		t.Fatalf("bind winning create identity: %v", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStatePending, AttemptStateCancelled); err != nil {
		t.Fatalf("terminal transition: %v", err)
	}
	replay, err := apply(winnerSessionID)
	if err != nil || replay.State != AttemptStateCancelled {
		t.Fatalf("exact terminal replay = %#v, %v; want cancelled no-op", replay, err)
	}
	if _, err := apply("session-terminal-create-loser-0001"); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("terminal identity mismatch error = %v; want ErrCaptureConflict", err)
	}
	stored, err := repo.GetAttemptByID(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != AttemptStateCancelled || stored.ProviderSessionID == nil || *stored.ProviderSessionID != winnerSessionID {
		t.Fatalf("terminal mismatch mutated attempt: %#v", stored)
	}
}

func TestApplyInquiryIdentityTerminalNoopRejectsBoundMismatch(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingID, "payment:create:terminal-inquiry-identity"))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "session-terminal-inquiry-winner-0001"
	requestID := "request-terminal-inquiry-winner-0001"
	paymentID := "payment-terminal-inquiry-winner-0001"
	applyCreateTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyCreateProviderResultTx(ctx, applyCreateTx, ApplyCreateProviderResultParams{
		AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment,
		ProviderSessionID: sessionID, ProviderStatusCode: "PENDING", Status: PaymentStatusPending,
		AmountRupiah: attempt.AmountRupiah, Currency: attempt.Currency,
	}); err != nil {
		_ = applyCreateTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := applyCreateTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	bind := func(params ApplyInquiryIdentityParams) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, _, err := repo.ApplyInquiryIdentityTx(ctx, tx, params); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	bind(ApplyInquiryIdentityParams{
		AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment,
		Scope: PaymentInquiryScopeCheckoutSession, ProviderSessionID: &sessionID,
		ProviderPaymentReqID: &requestID, ProviderStatusCode: "PENDING",
	})
	bind(ApplyInquiryIdentityParams{
		AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment,
		Scope: PaymentInquiryScopePayment, ProviderSessionID: &sessionID,
		ProviderPaymentReqID: &requestID, ProviderPaymentID: &paymentID, ProviderStatusCode: "PENDING",
	})
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStatePending, AttemptStateCancelled); err != nil {
		t.Fatal(err)
	}

	applyTerminal := func(params ApplyInquiryIdentityParams) (PaymentAttempt, error) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			return PaymentAttempt{}, err
		}
		defer tx.Rollback(ctx)
		current, _, err := repo.ApplyInquiryIdentityTx(ctx, tx, params)
		return current, err
	}
	exact, err := applyTerminal(ApplyInquiryIdentityParams{
		AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment,
		Scope: PaymentInquiryScopePayment, ProviderSessionID: &sessionID,
		ProviderPaymentReqID: &requestID, ProviderPaymentID: &paymentID, ProviderStatusCode: "FAILED",
	})
	if !errors.Is(err, ErrStateConflict) || exact.State != AttemptStateCancelled {
		t.Fatalf("exact terminal identity result = %#v, %v; want cancelled ErrStateConflict", exact, err)
	}

	wrongSession := "session-terminal-inquiry-loser-0001"
	wrongRequest := "request-terminal-inquiry-loser-0001"
	wrongPayment := "payment-terminal-inquiry-loser-0001"
	for _, params := range []ApplyInquiryIdentityParams{
		{AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment, Scope: PaymentInquiryScopeCheckoutSession, ProviderSessionID: &wrongSession, ProviderPaymentReqID: &requestID, ProviderStatusCode: "PENDING"},
		{AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment, Scope: PaymentInquiryScopeCheckoutSession, ProviderSessionID: &sessionID, ProviderPaymentReqID: &wrongRequest, ProviderStatusCode: "PENDING"},
		{AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment, Scope: PaymentInquiryScopePayment, ProviderSessionID: &sessionID, ProviderPaymentReqID: &wrongRequest, ProviderPaymentID: &paymentID, ProviderStatusCode: "FAILED"},
		{AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment, Scope: PaymentInquiryScopePayment, ProviderSessionID: &wrongSession, ProviderPaymentReqID: &requestID, ProviderPaymentID: &paymentID, ProviderStatusCode: "FAILED"},
		{AttemptID: attempt.ID, Provider: attempt.Provider, ProviderEnvironment: attempt.ProviderEnvironment, Scope: PaymentInquiryScopePayment, ProviderSessionID: &sessionID, ProviderPaymentReqID: &requestID, ProviderPaymentID: &wrongPayment, ProviderStatusCode: "FAILED"},
	} {
		if _, err := applyTerminal(params); !errors.Is(err, ErrCaptureConflict) {
			t.Fatalf("terminal identity mismatch %#v error = %v; want ErrCaptureConflict", params, err)
		}
	}
}

func TestApplyInquiryIdentityRejectsUnsafeInputBeforeQuery(t *testing.T) {
	repo := &Repository{}
	if _, _, err := repo.ApplyInquiryIdentityTx(context.Background(), nil, ApplyInquiryIdentityParams{}); !errors.Is(err, ErrInvalidInquiryIdentity) {
		t.Fatalf("nil transaction identity error = %v; want ErrInvalidInquiryIdentity", err)
	}
	if _, _, err := repo.ApplyInquiryIdentityTx(context.Background(), nil, ApplyInquiryIdentityParams{AttemptID: "not-a-uuid", Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest, Scope: PaymentInquiryScopePayment, ProviderStatusCode: "PENDING"}); !errors.Is(err, ErrInvalidInquiryIdentity) {
		t.Fatalf("unsafe identity error = %v; want ErrInvalidInquiryIdentity", err)
	}
}

func TestRepositoryLateCaptureAndMismatchRollback(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)

	withoutSnapshot := seedRepositoryBooking(t, ctx, pool, false)
	if _, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(withoutSnapshot, "payment:create:no-snapshot")); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("missing snapshot error = %v; want ErrSnapshotNotFound", err)
	}

	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingID, "payment:create:late"))
	if err != nil {
		t.Fatalf("create late attempt: %v", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("late attempt to pending: %v", err)
	}
	if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStatePending, AttemptStateExpired); err != nil {
		t.Fatalf("late attempt to expired: %v", err)
	}

	captureTime := time.Now().UTC().Add(-time.Second)
	late, err := repo.RecordCapture(ctx, validCaptureParams(attempt.ID, "late-payment", captureTime))
	if err != nil {
		t.Fatalf("record late capture: %v", err)
	}
	if !late.LateCapture || late.Attempt.State != AttemptStateCaptured {
		t.Fatalf("late capture result = %#v; want late captured", late)
	}

	bookingMismatch := seedRepositoryBooking(t, ctx, pool, true)
	mismatchAttempt, err := repo.CreateOrReplayAttempt(ctx, validCreateParams(bookingMismatch, "payment:create:mismatch"))
	if err != nil {
		t.Fatalf("create mismatch attempt: %v", err)
	}
	if _, err := repo.TransitionState(ctx, mismatchAttempt.ID, AttemptStateCreated, AttemptStatePending); err != nil {
		t.Fatalf("mismatch attempt to pending: %v", err)
	}
	mismatch := validCaptureParams(mismatchAttempt.ID, "mismatch-payment", captureTime)
	mismatch.AmountRupiah = 9999
	if _, err := repo.RecordCapture(ctx, mismatch); !errors.Is(err, ErrCaptureConflict) {
		t.Fatalf("amount mismatch error = %v; want ErrCaptureConflict", err)
	}
	current, err := repo.GetAttemptByID(ctx, mismatchAttempt.ID)
	if err != nil {
		t.Fatalf("get attempt after mismatch: %v", err)
	}
	if current.State != AttemptStatePending || current.CapturedAt != nil {
		t.Fatalf("mismatch changed attempt: %#v", current)
	}
	var facts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_capture_facts WHERE payment_attempt_id = $1`, mismatchAttempt.ID).Scan(&facts); err != nil {
		t.Fatalf("count mismatch facts: %v", err)
	}
	if facts != 0 {
		t.Fatalf("mismatch created %d capture facts; want 0", facts)
	}
}

func TestRepositoryMarksPendingCaptureLateAfterBookingCancellationOrExpiry(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	repo := NewRepository(pool)

	tests := []struct {
		name       string
		localState func(string) error
	}{
		{
			name: "booking cancelled",
			localState: func(bookingID string) error {
				_, err := pool.Exec(ctx, `UPDATE bookings SET status = 'CANCELLED' WHERE id = $1`, bookingID)
				return err
			},
		},
		{
			name: "booking expired before sweep",
			localState: func(bookingID string) error {
				_, err := pool.Exec(ctx, `
					UPDATE bookings
					SET expires_at = transaction_timestamp() - interval '1 second'
					WHERE id = $1
				`, bookingID)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bookingID := seedRepositoryBooking(t, ctx, pool, true)
			attempt, err := repo.CreateOrReplayAttempt(
				ctx,
				validCreateParams(bookingID, "payment:create:booking-late:"+strings.ReplaceAll(uuid.NewString(), "-", "")),
			)
			if err != nil {
				t.Fatalf("create attempt: %v", err)
			}
			if _, err := repo.TransitionState(ctx, attempt.ID, AttemptStateCreated, AttemptStatePending); err != nil {
				t.Fatalf("transition attempt to pending: %v", err)
			}
			if err := test.localState(bookingID); err != nil {
				t.Fatalf("apply local booking terminal state: %v", err)
			}

			capturedAt := time.Now().UTC().Truncate(time.Microsecond)
			result, err := repo.RecordCapture(
				ctx,
				validCaptureParams(attempt.ID, "booking-late-"+strings.ReplaceAll(uuid.NewString(), "-", ""), capturedAt),
			)
			if err != nil {
				t.Fatalf("record capture after local booking terminal state: %v", err)
			}
			if !result.LateCapture || result.Attempt.State != AttemptStateCaptured {
				t.Fatalf("capture result = %#v; want late captured", result)
			}

			var exceptionCount int
			if err := pool.QueryRow(ctx, `
				SELECT count(*)
				FROM platform_audit_logs
				WHERE entity_id = $1
				  AND action = $2
				  AND metadata->>'reason' = 'LATE_CAPTURE'
			`, attempt.ID, audit.ActionReconciliationException).Scan(&exceptionCount); err != nil {
				t.Fatalf("count late-capture reconciliation audit: %v", err)
			}
			if exceptionCount != 1 {
				t.Fatalf("late-capture reconciliation audit count = %d; want 1", exceptionCount)
			}
		})
	}
}

func openPaymentTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	if os.Getenv("TEST_ROLLBACK_HARDENING_DISPOSABLE") != "1" {
		t.Skip("disposable payment repository test is opt-in")
	}
	adminDSN := os.Getenv("ROLLBACK_HARDENING_TEST_DATABASE_URL")
	if adminDSN == "" {
		t.Fatal("ROLLBACK_HARDENING_TEST_DATABASE_URL is required")
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil || parsed.Path == "" || parsed.Path == "/" {
		t.Fatalf("invalid admin DSN")
	}
	dbName := "lapangango_payment_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	adminDB, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if _, err := adminDB.Exec("CREATE DATABASE " + dbName); err != nil {
		adminDB.Close()
		t.Fatalf("create disposable database: %v", err)
	}
	adminDB.Close()
	parsed.Path = "/" + dbName
	targetDSN := parsed.String()
	var pool *pgxpool.Pool
	err = initializePaymentDisposableAfterCreate(
		t.Cleanup,
		func() {
			if pool != nil {
				pool.Close()
			}
			cleanupDB, cleanupErr := sql.Open("postgres", adminDSN)
			if cleanupErr == nil {
				defer cleanupDB.Close()
				_, _ = cleanupDB.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, dbName)
				_, _ = cleanupDB.Exec("DROP DATABASE " + dbName)
			}
		},
		func() error {
			targetDB, setupErr := sql.Open("postgres", targetDSN)
			if setupErr != nil {
				return fmt.Errorf("open target database: %w", setupErr)
			}
			defer targetDB.Close()
			driver, setupErr := postgres.WithInstance(targetDB, &postgres.Config{})
			if setupErr != nil {
				return fmt.Errorf("create migration driver: %w", setupErr)
			}
			m, setupErr := migrate.NewWithDatabaseInstance("file://../../../../db/migrations", "postgres", driver)
			if setupErr != nil {
				return fmt.Errorf("create migration runner: %w", setupErr)
			}
			if setupErr = m.Up(); setupErr != nil && setupErr != migrate.ErrNoChange {
				_, _ = m.Close()
				return fmt.Errorf("run migrations: %w", setupErr)
			}
			_, _ = m.Close()

			pool, setupErr = pgxpool.New(context.Background(), targetDSN)
			if setupErr != nil {
				return fmt.Errorf("open target pool: %w", setupErr)
			}
			if setupErr = pool.Ping(context.Background()); setupErr != nil {
				pool.Close()
				pool = nil
				return fmt.Errorf("ping target pool: %w", setupErr)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("initialize disposable payment database: %v", err)
	}
	return context.Background(), pool
}

func initializePaymentDisposableAfterCreate(
	registerCleanup func(func()),
	cleanup func(),
	initialize func() error,
) error {
	registerCleanup(cleanup)
	return initialize()
}

func seedRepositoryBooking(t *testing.T, ctx context.Context, pool *pgxpool.Pool, withSnapshot bool) string {
	t.Helper()
	customerID := uuid.NewString()
	ownerUserID := uuid.NewString()
	ownerProfileID := uuid.NewString()
	venueID := uuid.NewString()
	courtID := uuid.NewString()
	bookingID := uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash, role, status)
		VALUES ($1, 'payment repository customer', $2, 'hash', 'CUSTOMER', 'ACTIVE'),
		       ($3, 'payment repository owner', $4, 'hash', 'OWNER', 'ACTIVE')
	`, customerID, "customer-"+suffix+"@example.test", ownerUserID, "owner-"+suffix+"@example.test"); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO owner_profiles (id, user_id, business_name, verification_status) VALUES ($1, $2, 'Payment Repository Owner', 'APPROVED')`, ownerProfileID, ownerUserID); err != nil {
		t.Fatalf("seed owner profile: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO venues (id, owner_profile_id, name, address, city, status) VALUES ($1, $2, $3, 'Test address', 'Jakarta', 'ACTIVE')`, venueID, ownerProfileID, "Payment Repository Venue "+suffix); err != nil {
		t.Fatalf("seed venue: %v", err)
	}
	var sportID string
	if err := pool.QueryRow(ctx, `SELECT id FROM sports WHERE name = 'Futsal' LIMIT 1`).Scan(&sportID); err != nil {
		t.Fatalf("find sport: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO courts (id, venue_id, sport_id, name, location_type, price_per_hour, status) VALUES ($1, $2, $3, $4, 'INDOOR', 10000, 'ACTIVE')`, courtID, venueID, sportID, "Payment Repository Court "+suffix); err != nil {
		t.Fatalf("seed court: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bookings (id, customer_id, court_id, booking_date, start_time, end_time, total_price, status, expires_at) VALUES ($1, $2, $3, CURRENT_DATE + 1, '10:00', '11:00', 10000, 'PENDING_PAYMENT', transaction_timestamp() + interval '2 hours')`, bookingID, customerID, courtID); err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	if !withSnapshot {
		return bookingID
	}
	var termID string
	if err := pool.QueryRow(ctx, `SELECT id FROM platform_commercial_terms WHERE owner_profile_id IS NULL LIMIT 1`).Scan(&termID); err != nil {
		t.Fatalf("find commercial term: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO booking_fee_snapshots (
			booking_id, owner_profile_id, venue_id, commercial_term_id, terms_source,
			booking_channel, finance_mode, original_price_rupiah, owner_price_adjustment_rupiah,
			final_booking_price_rupiah, customer_charge_amount_rupiah, commission_basis_amount_rupiah,
			commission_bps, commission_amount_rupiah, owner_net_amount_rupiah, calculation_version
		) VALUES ($1, $2, $3, $4, 'POLICY', 'MARKETPLACE_ONLINE', 'SIMULATION',
			10000, 0, 10000, 10000, 10000, 700, 700, 9300, 'PAYMENT_REPOSITORY_TEST_V1')
	`, bookingID, ownerProfileID, venueID, termID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	return bookingID
}

func validCreateParams(bookingID, localReference string) CreateAttemptParams {
	return CreateAttemptParams{
		BookingID: bookingID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		RequestedMethod: RequestedMethodQRIS, IntegrationMode: IntegrationModePaymentLink,
		CaptureMethod: CaptureMethodAutomatic, LocalReference: localReference,
		RequestHash: validPaymentHash, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
}

func validCaptureParams(attemptID, providerPaymentID string, capturedAt time.Time) CaptureParams {
	return CaptureParams{
		AttemptID: attemptID, Provider: ProviderXendit, ProviderEnvironment: ProviderEnvironmentTest,
		ProviderPaymentID: providerPaymentID, AmountRupiah: 10000, Currency: CurrencyIDR,
		CapturedAt: capturedAt, ObservedAt: capturedAt.Add(time.Second), Authority: "AUTHENTICATED_INQUIRY",
		SourceReference: "inquiry:" + providerPaymentID, PayloadHash: validPaymentHash,
	}
}

func TestAllowedStateTransition(t *testing.T) {
	legal := []struct {
		from AttemptState
		to   AttemptState
	}{
		{AttemptStateCreated, AttemptStatePending},
		{AttemptStateCreated, AttemptStateCancelled},
		{AttemptStatePending, AttemptStateFailed},
		{AttemptStatePending, AttemptStateExpired},
		{AttemptStatePending, AttemptStateCancelled},
	}
	for _, tc := range legal {
		if !allowedStateTransition(tc.from, tc.to) {
			t.Errorf("allowedStateTransition(%q, %q) = false", tc.from, tc.to)
		}
	}

	denied := []struct {
		from AttemptState
		to   AttemptState
	}{
		{AttemptStateCreated, AttemptStateCaptured},
		{AttemptStatePending, AttemptStateCaptured},
		{AttemptStateCaptured, AttemptStatePending},
		{AttemptStateCaptured, AttemptStateFailed},
		{AttemptStateFailed, AttemptStatePending},
		{AttemptStateExpired, AttemptStatePending},
		{AttemptStateCancelled, AttemptStatePending},
	}
	for _, tc := range denied {
		if allowedStateTransition(tc.from, tc.to) {
			t.Errorf("allowedStateTransition(%q, %q) = true", tc.from, tc.to)
		}
	}
}
