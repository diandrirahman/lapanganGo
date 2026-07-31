package payments

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"lapangango-api/internal/audit"
	"lapangango-api/internal/bookings"
	"lapangango-api/internal/paymentoutbox"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSandboxCreateOrchestrationAtomicReplayAndOwnership(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	var customerID string
	if err := pool.QueryRow(ctx, `SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatalf("read seeded customer: %v", err)
	}
	platformAudit := audit.NewPlatformService(audit.NewPlatformRepository())
	orchestrator := NewOrchestrator(pool, NewRepository(pool), paymentoutbox.NewRepository(pool), platformAudit, OrchestratorOptions{
		SandboxEnabled: true,
		CreateEnabled:  true,
		AttemptTTL:     time.Hour,
		ReturnOrigin:   "https://demo.example.test",
	})

	first, err := orchestrator.CreatePayment(ctx, customerID, strings.ToUpper(bookingID), "client-retry-1", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if first.Replay || first.Attempt.State != AttemptStateCreated || first.Attempt.AmountRupiah != 10000 {
		t.Fatalf("unexpected first result: %#v", first)
	}

	var attemptCount, commandCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, bookingID).Scan(&attemptCount); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_provider_commands WHERE payment_attempt_id = $1`, first.Attempt.ID).Scan(&commandCount); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE entity_id = $1`, first.Attempt.ID).Scan(&auditCount); err != nil {
		t.Fatalf("count payment audits: %v", err)
	}
	if attemptCount != 1 || commandCount != 1 || auditCount != 2 {
		t.Fatalf("atomic create counts attempts=%d commands=%d audits=%d; want 1/1/2", attemptCount, commandCount, auditCount)
	}

	replay, err := orchestrator.CreatePayment(ctx, customerID, bookingID, "client-retry-1", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
	if err != nil || !replay.Replay || replay.Attempt.ID != first.Attempt.ID {
		t.Fatalf("same-key replay = %#v, %v", replay, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, bookingID).Scan(&attemptCount); err != nil {
		t.Fatalf("count replay attempts: %v", err)
	}
	if attemptCount != 1 {
		t.Fatalf("replay created %d attempts", attemptCount)
	}

	if _, err := orchestrator.CreatePayment(ctx, customerID, bookingID, "client-retry-1", CreateAttemptRequest{RequestedMethod: RequestedMethodCard}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key/different method error = %v; want conflict", err)
	}
	if _, err := orchestrator.CreatePayment(ctx, uuid.NewString(), bookingID, "foreign-key", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS}); !errors.Is(err, ErrStateConflict) && !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("foreign active attempt error = %v; want safe rejection", err)
	}

	view, err := orchestrator.GetPaymentAttempt(ctx, customerID, first.Attempt.ID)
	if err != nil || view.CheckoutURL != nil || view.State != AttemptStateCreated {
		t.Fatalf("safe status view = %#v, %v", view, err)
	}
	if _, err := orchestrator.GetPaymentAttempt(ctx, uuid.NewString(), first.Attempt.ID); !errors.Is(err, ErrPaymentAccessDenied) {
		t.Fatalf("foreign status read error = %v; want not found", err)
	}

	var bookingStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, bookingID).Scan(&bookingStatus); err != nil {
		t.Fatalf("read booking status: %v", err)
	}
	if bookingStatus != "PENDING_PAYMENT" {
		t.Fatalf("booking status changed to %q", bookingStatus)
	}

	disabledBookingID := seedRepositoryBooking(t, ctx, pool, true)
	var disabledCustomerID string
	if err := pool.QueryRow(ctx, `SELECT customer_id::text FROM bookings WHERE id = $1`, disabledBookingID).Scan(&disabledCustomerID); err != nil {
		t.Fatalf("read disabled customer: %v", err)
	}
	disabled := NewOrchestrator(pool, NewRepository(pool), paymentoutbox.NewRepository(pool), platformAudit, OrchestratorOptions{})
	if _, err := disabled.CreatePayment(ctx, disabledCustomerID, disabledBookingID, "flag-off-request", CreateAttemptRequest{RequestedMethod: RequestedMethodBCAVA}); !errors.Is(err, ErrPaymentCapabilityDisabled) {
		t.Fatalf("flag-off create error = %v; want disabled", err)
	}
	var disabledAttemptCount, disabledAuditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, disabledBookingID).Scan(&disabledAttemptCount); err != nil {
		t.Fatalf("count disabled attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE action = $1 AND actor_user_id = $2`, audit.ActionPaymentCreateFlagOffRejected, disabledCustomerID).Scan(&disabledAuditCount); err != nil {
		t.Fatalf("count disabled audits: %v", err)
	}
	if disabledAttemptCount != 0 || disabledAuditCount != 1 {
		t.Fatalf("flag-off counts attempts=%d audits=%d; want 0/1", disabledAttemptCount, disabledAuditCount)
	}
}

func TestPaymentAttemptStatusHidesCheckoutWhenBookingOrAttemptIsNoLongerPayable(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	orchestrator := NewOrchestrator(
		pool,
		NewRepository(pool),
		paymentoutbox.NewRepository(pool),
		audit.NewPlatformService(audit.NewPlatformRepository()),
		OrchestratorOptions{
			SandboxEnabled: true,
			CreateEnabled:  true,
			AttemptTTL:     time.Hour,
			ReturnOrigin:   "https://demo.example.test",
		},
	)
	checkoutURL := "https://checkout-staging.xendit.co/sessions/ps-status-eligibility"

	makePendingAttempt := func(key string) (string, string, string) {
		t.Helper()
		bookingID, customerID := seedOrchestrationBooking(t, ctx, pool)
		created, err := orchestrator.CreatePayment(
			ctx,
			customerID,
			bookingID,
			key,
			CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS},
		)
		if err != nil {
			t.Fatalf("create %s payment: %v", key, err)
		}
		if _, err := pool.Exec(ctx, `
			UPDATE payment_attempts
			SET state = 'PENDING',
			    provider_session_id = $2,
			    provider_status_code = 'PENDING',
			    checkout_url = $3,
			    updated_at = transaction_timestamp()
			WHERE id = $1
		`, created.Attempt.ID, "session-"+key, checkoutURL); err != nil {
			t.Fatalf("mark %s payment pending: %v", key, err)
		}
		return bookingID, customerID, created.Attempt.ID
	}

	cancelledBookingID, cancelledCustomerID, cancelledAttemptID := makePendingAttempt("status-cancelled")
	cancelledReference := deterministicLocalReference(cancelledBookingID, "status-cancelled")
	beforeCancellation, err := orchestrator.GetPaymentAttempt(ctx, cancelledCustomerID, cancelledAttemptID)
	if err != nil || beforeCancellation.CheckoutURL == nil {
		t.Fatalf("eligible checkout before cancellation = %#v, %v; want URL", beforeCancellation, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bookings
		SET status = 'CANCELLED', updated_at = transaction_timestamp()
		WHERE id = $1
	`, cancelledBookingID); err != nil {
		t.Fatalf("cancel booking with pending attempt: %v", err)
	}
	for name, read := range map[string]func() (PaymentAttemptView, error){
		"id": func() (PaymentAttemptView, error) {
			return orchestrator.GetPaymentAttempt(ctx, cancelledCustomerID, cancelledAttemptID)
		},
		"reference": func() (PaymentAttemptView, error) {
			return orchestrator.GetPaymentAttemptByReference(ctx, cancelledCustomerID, cancelledReference)
		},
	} {
		view, err := read()
		if err != nil || view.State != AttemptStatePending || view.CheckoutURL != nil {
			t.Fatalf("%s status after booking cancellation = %#v, %v; want pending without checkout", name, view, err)
		}
	}

	expiredBookingID, expiredCustomerID, expiredAttemptID := makePendingAttempt("status-booking-expired")
	if _, err := pool.Exec(ctx, `
		UPDATE bookings
		SET expires_at = transaction_timestamp() - interval '1 second',
		    updated_at = transaction_timestamp()
		WHERE id = $1
	`, expiredBookingID); err != nil {
		t.Fatalf("expire booking before status read: %v", err)
	}
	expiredBookingView, err := orchestrator.GetPaymentAttempt(ctx, expiredCustomerID, expiredAttemptID)
	if err != nil || expiredBookingView.State != AttemptStatePending || expiredBookingView.CheckoutURL != nil {
		t.Fatalf("status after booking expiry = %#v, %v; want pending without checkout", expiredBookingView, err)
	}

	attemptExpiryBookingID, attemptExpiryCustomerID, attemptExpiryAttemptID := makePendingAttempt("status-attempt-expired")
	if _, err := pool.Exec(ctx, `
		UPDATE payment_attempts
		SET expires_at = transaction_timestamp() + interval '200 milliseconds',
		    updated_at = transaction_timestamp()
		WHERE id = $1
	`, attemptExpiryAttemptID); err != nil {
		t.Fatalf("shorten attempt expiry: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	expiredAttemptView, err := orchestrator.GetPaymentAttempt(ctx, attemptExpiryCustomerID, attemptExpiryAttemptID)
	if err != nil || expiredAttemptView.State != AttemptStatePending || expiredAttemptView.CheckoutURL != nil {
		t.Fatalf("status after attempt expiry = %#v, %v; want pending without checkout", expiredAttemptView, err)
	}

	var bookingStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, attemptExpiryBookingID).Scan(&bookingStatus); err != nil {
		t.Fatalf("read attempt-expiry booking status: %v", err)
	}
	if bookingStatus != "PENDING_PAYMENT" {
		t.Fatalf("attempt-expiry booking status = %q; want PENDING_PAYMENT", bookingStatus)
	}
}

func TestSandboxCreateRejectsExpiredBookingAndBoundsAttemptExpiry(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	platformAudit := audit.NewPlatformService(audit.NewPlatformRepository())
	orchestrator := NewOrchestrator(pool, NewRepository(pool), paymentoutbox.NewRepository(pool), platformAudit, OrchestratorOptions{
		SandboxEnabled: true,
		CreateEnabled:  true,
		AttemptTTL:     time.Hour,
		ReturnOrigin:   "https://demo.example.test",
	})

	expiredBookingID := seedRepositoryBooking(t, ctx, pool, true)
	var expiredCustomerID string
	if err := pool.QueryRow(ctx, `
		UPDATE bookings
		SET expires_at = transaction_timestamp() - interval '1 second'
		WHERE id = $1
		RETURNING customer_id::text
	`, expiredBookingID).Scan(&expiredCustomerID); err != nil {
		t.Fatalf("expire booking: %v", err)
	}
	if _, err := orchestrator.CreatePayment(ctx, expiredCustomerID, expiredBookingID, "expired-booking", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS}); !errors.Is(err, ErrBookingNotPayable) {
		t.Fatalf("expired booking error = %v; want ErrBookingNotPayable", err)
	}
	var expiredAttemptCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, expiredBookingID).Scan(&expiredAttemptCount); err != nil {
		t.Fatalf("count expired booking attempts: %v", err)
	}
	if expiredAttemptCount != 0 {
		t.Fatalf("expired booking created %d attempts", expiredAttemptCount)
	}

	nearExpiryBookingID := seedRepositoryBooking(t, ctx, pool, true)
	var nearExpiryCustomerID string
	var bookingExpiresAt time.Time
	if err := pool.QueryRow(ctx, `
		UPDATE bookings
		SET expires_at = transaction_timestamp() + interval '2 minutes'
		WHERE id = $1
		RETURNING customer_id::text, expires_at
	`, nearExpiryBookingID).Scan(&nearExpiryCustomerID, &bookingExpiresAt); err != nil {
		t.Fatalf("set near expiry booking: %v", err)
	}
	result, err := orchestrator.CreatePayment(ctx, nearExpiryCustomerID, nearExpiryBookingID, "near-expiry-booking", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
	if err != nil {
		t.Fatalf("create near-expiry payment: %v", err)
	}
	if !result.Attempt.ExpiresAt.Equal(bookingExpiresAt) {
		t.Fatalf("attempt expiry = %s; want booking expiry %s", result.Attempt.ExpiresAt, bookingExpiresAt)
	}
}

func TestSandboxCreateReplaysOriginalAfterBookingBecomesTerminal(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	var customerID string
	if err := pool.QueryRow(ctx, `SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatalf("read seeded customer: %v", err)
	}
	orchestrator := NewOrchestrator(
		pool,
		NewRepository(pool),
		paymentoutbox.NewRepository(pool),
		audit.NewPlatformService(audit.NewPlatformRepository()),
		OrchestratorOptions{
			SandboxEnabled: true,
			CreateEnabled:  true,
			AttemptTTL:     time.Hour,
			ReturnOrigin:   "https://demo.example.test",
		},
	)

	first, err := orchestrator.CreatePayment(ctx, customerID, bookingID, "terminal-replay", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
	if err != nil {
		t.Fatalf("create original payment: %v", err)
	}
	if _, err := bookings.NewRepository(pool).CancelPendingByIDAndCustomerID(ctx, bookingID, customerID); err != nil {
		t.Fatalf("cancel booking through sandbox-aware path: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bookings
		SET expires_at = transaction_timestamp() - interval '1 second'
		WHERE id = $1
	`, bookingID); err != nil {
		t.Fatalf("expire terminal booking: %v", err)
	}

	replay, err := orchestrator.CreatePayment(ctx, customerID, bookingID, "terminal-replay", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
	if err != nil || !replay.Replay || replay.Attempt.ID != first.Attempt.ID {
		t.Fatalf("terminal booking replay = %#v, %v; want original attempt", replay, err)
	}
	if _, err := orchestrator.CreatePayment(ctx, customerID, bookingID, "terminal-replay", CreateAttemptRequest{RequestedMethod: RequestedMethodCard}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("terminal booking different-payload error = %v; want ErrIdempotencyConflict", err)
	}

	var attemptCount, commandCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, bookingID).Scan(&attemptCount); err != nil {
		t.Fatalf("count replay attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_provider_commands WHERE payment_attempt_id = $1`, first.Attempt.ID).Scan(&commandCount); err != nil {
		t.Fatalf("count replay commands: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_logs WHERE entity_id = $1`, first.Attempt.ID).Scan(&auditCount); err != nil {
		t.Fatalf("count replay audits: %v", err)
	}
	if attemptCount != 1 || commandCount != 1 || auditCount != 3 {
		t.Fatalf("terminal replay counts attempts=%d commands=%d audits=%d; want 1/1/3", attemptCount, commandCount, auditCount)
	}
}

func TestSandboxCreateReplayUsesImmutableContractAfterRuntimeDrift(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	var customerID string
	if err := pool.QueryRow(ctx, `SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatalf("read seeded customer: %v", err)
	}
	repository := NewRepository(pool)
	orchestrator := NewOrchestrator(
		pool,
		repository,
		paymentoutbox.NewRepository(pool),
		audit.NewPlatformService(audit.NewPlatformRepository()),
		OrchestratorOptions{
			SandboxEnabled: true,
			CreateEnabled:  true,
			AttemptTTL:     time.Hour,
			ReturnOrigin:   "https://demo.example.test",
		},
	)

	first, err := orchestrator.CreatePayment(ctx, customerID, bookingID, "immutable-create-facts", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
	if err != nil {
		t.Fatalf("create original payment: %v", err)
	}
	contract, err := repository.GetCreateContractByAttemptID(ctx, first.Attempt.ID)
	if err != nil {
		t.Fatalf("read original create contract: %v", err)
	}
	originalHash := first.Attempt.RequestHash
	originalRequestedExpiry := contract.RequestedExpiresAt
	originalSuccessURL := contract.SuccessReturnURL
	originalCancelURL := contract.CancelReturnURL

	var providerExpiry time.Time
	if err := pool.QueryRow(ctx, `
		UPDATE payment_attempts
		SET expires_at = expires_at - interval '1 minute',
		    updated_at = transaction_timestamp()
		WHERE id = $1
		RETURNING expires_at
	`, first.Attempt.ID).Scan(&providerExpiry); err != nil {
		t.Fatalf("simulate provider expiry normalization: %v", err)
	}
	orchestrator.options.ReturnOrigin = "https://changed.example.test"

	replay, err := orchestrator.CreatePayment(ctx, customerID, bookingID, "immutable-create-facts", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
	if err != nil || !replay.Replay || replay.Attempt.ID != first.Attempt.ID {
		t.Fatalf("runtime-drift replay = %#v, %v; want original attempt", replay, err)
	}
	if replay.Attempt.RequestHash != originalHash || !replay.Attempt.ExpiresAt.Equal(providerExpiry) {
		t.Fatalf("replay attempt hash/expiry = %q/%s; want %q/%s", replay.Attempt.RequestHash, replay.Attempt.ExpiresAt, originalHash, providerExpiry)
	}
	storedContract, err := repository.GetCreateContractByAttemptID(ctx, first.Attempt.ID)
	if err != nil {
		t.Fatalf("read replay create contract: %v", err)
	}
	if !storedContract.RequestedExpiresAt.Equal(originalRequestedExpiry) ||
		storedContract.SuccessReturnURL != originalSuccessURL ||
		storedContract.CancelReturnURL != originalCancelURL {
		t.Fatalf("immutable create contract drifted: %#v", storedContract)
	}

	localReference := deterministicLocalReference(bookingID, "immutable-create-facts")
	view, err := orchestrator.GetPaymentAttemptByReference(ctx, customerID, localReference)
	if err != nil || view.ID != first.Attempt.ID || !view.ExpiresAt.Equal(providerExpiry) {
		t.Fatalf("safe return-reference resolution = %#v, %v", view, err)
	}
	if _, err := orchestrator.GetPaymentAttemptByReference(ctx, uuid.NewString(), localReference); !errors.Is(err, ErrPaymentAccessDenied) {
		t.Fatalf("foreign return-reference resolution error = %v; want access denied", err)
	}
}

func TestSandboxCreateConcurrencyMatrix(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	platformAudit := audit.NewPlatformService(audit.NewPlatformRepository())
	newTestOrchestrator := func() *Orchestrator {
		return NewOrchestrator(
			pool,
			NewRepository(pool),
			paymentoutbox.NewRepository(pool),
			platformAudit,
			OrchestratorOptions{
				SandboxEnabled: true,
				CreateEnabled:  true,
				AttemptTTL:     time.Hour,
				ReturnOrigin:   "https://demo.example.test",
			},
		)
	}

	t.Run("same key and same payload", func(t *testing.T) {
		bookingID, customerID := seedOrchestrationBooking(t, ctx, pool)
		orchestrator := newTestOrchestrator()
		results := runConcurrentCreates(
			func() (CreatePaymentResult, error) {
				return orchestrator.CreatePayment(ctx, customerID, strings.ToUpper(bookingID), "concurrent-same", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
			},
			func() (CreatePaymentResult, error) {
				return orchestrator.CreatePayment(ctx, customerID, bookingID, "concurrent-same", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
			},
		)
		if results[0].err != nil || results[1].err != nil {
			t.Fatalf("concurrent same-payload errors = %v / %v", results[0].err, results[1].err)
		}
		if results[0].result.Attempt.ID != results[1].result.Attempt.ID ||
			results[0].result.Replay == results[1].result.Replay {
			t.Fatalf("concurrent same-payload results = %#v / %#v; want one fresh and one replay", results[0].result, results[1].result)
		}
		assertOrchestrationRowCounts(t, ctx, pool, bookingID, 1, 1, 1, 2)
	})

	t.Run("same key and different payload", func(t *testing.T) {
		bookingID, customerID := seedOrchestrationBooking(t, ctx, pool)
		orchestrator := newTestOrchestrator()
		results := runConcurrentCreates(
			func() (CreatePaymentResult, error) {
				return orchestrator.CreatePayment(ctx, customerID, bookingID, "concurrent-conflict", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
			},
			func() (CreatePaymentResult, error) {
				return orchestrator.CreatePayment(ctx, customerID, bookingID, "concurrent-conflict", CreateAttemptRequest{RequestedMethod: RequestedMethodCard})
			},
		)
		successes, conflicts := 0, 0
		for _, result := range results {
			if result.err == nil {
				successes++
			} else if errors.Is(result.err, ErrIdempotencyConflict) {
				conflicts++
			} else {
				t.Fatalf("unexpected concurrent conflict error: %v", result.err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("same-key different-payload successes/conflicts = %d/%d; want 1/1", successes, conflicts)
		}
		assertOrchestrationRowCounts(t, ctx, pool, bookingID, 1, 1, 1, 2)
	})

	t.Run("different keys for one booking", func(t *testing.T) {
		bookingID, customerID := seedOrchestrationBooking(t, ctx, pool)
		orchestrator := newTestOrchestrator()
		results := runConcurrentCreates(
			func() (CreatePaymentResult, error) {
				return orchestrator.CreatePayment(ctx, customerID, bookingID, "concurrent-first", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
			},
			func() (CreatePaymentResult, error) {
				return orchestrator.CreatePayment(ctx, customerID, bookingID, "concurrent-second", CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS})
			},
		)
		successes, stateConflicts := 0, 0
		for _, result := range results {
			if result.err == nil {
				successes++
			} else if errors.Is(result.err, ErrStateConflict) {
				stateConflicts++
			} else {
				t.Fatalf("unexpected different-key concurrency error: %v", result.err)
			}
		}
		if successes != 1 || stateConflicts != 1 {
			t.Fatalf("different-key successes/state-conflicts = %d/%d; want 1/1", successes, stateConflicts)
		}
		assertOrchestrationRowCounts(t, ctx, pool, bookingID, 1, 1, 1, 2)
	})
}

func TestSandboxCreateRollsBackAttemptOutboxAndAuditOnLateFailure(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	var customerID string
	if err := pool.QueryRow(ctx, `SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatalf("read seeded customer: %v", err)
	}
	injected := errors.New("injected command audit failure")
	failingOrchestrator := NewOrchestrator(pool, NewRepository(pool), paymentoutbox.NewRepository(pool), failingPlatformAudit{err: injected}, OrchestratorOptions{
		SandboxEnabled: true,
		CreateEnabled:  true,
		AttemptTTL:     time.Hour,
		ReturnOrigin:   "https://demo.example.test",
	})
	if _, err := failingOrchestrator.CreatePayment(ctx, customerID, bookingID, "rollback-proof", CreateAttemptRequest{RequestedMethod: RequestedMethodBCAVA}); !errors.Is(err, injected) {
		t.Fatalf("injected orchestration error = %v; want %v", err, injected)
	}

	var attemptCount, commandCount, auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, bookingID).Scan(&attemptCount); err != nil {
		t.Fatalf("count rolled-back attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_provider_commands`).Scan(&commandCount); err != nil {
		t.Fatalf("count rolled-back commands: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM platform_audit_logs
		WHERE actor_user_id = $1
		  AND action IN ($2, $3)
	`, customerID, audit.ActionPaymentAttemptCreated, audit.ActionPaymentCommandEnqueued).Scan(&auditCount); err != nil {
		t.Fatalf("count rolled-back audits: %v", err)
	}
	if attemptCount != 0 || commandCount != 0 || auditCount != 0 {
		t.Fatalf("rollback counts attempts=%d commands=%d audits=%d; want 0/0/0", attemptCount, commandCount, auditCount)
	}

	retryOrchestrator := NewOrchestrator(pool, NewRepository(pool), paymentoutbox.NewRepository(pool), audit.NewPlatformService(audit.NewPlatformRepository()), OrchestratorOptions{
		SandboxEnabled: true,
		CreateEnabled:  true,
		AttemptTTL:     time.Hour,
		ReturnOrigin:   "https://demo.example.test",
	})
	retry, err := retryOrchestrator.CreatePayment(ctx, customerID, bookingID, "rollback-proof", CreateAttemptRequest{RequestedMethod: RequestedMethodBCAVA})
	if err != nil || retry.Replay {
		t.Fatalf("retry after rollback = %#v, %v; want fresh success", retry, err)
	}
}

func TestSandboxCreateIsolatesLegacyFlowAndCancellationNeutralizesCommand(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	bookingID, customerID := seedOrchestrationBooking(t, ctx, pool)
	outbox := paymentoutbox.NewRepository(pool)
	orchestrator := NewOrchestrator(
		pool,
		NewRepository(pool),
		outbox,
		audit.NewPlatformService(audit.NewPlatformRepository()),
		OrchestratorOptions{
			SandboxEnabled: true,
			CreateEnabled:  true,
			AttemptTTL:     time.Hour,
			ReturnOrigin:   "https://demo.example.test",
		},
	)
	created, err := orchestrator.CreatePayment(
		ctx,
		customerID,
		bookingID,
		"isolation-and-cancel",
		CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS},
	)
	if err != nil {
		t.Fatalf("create sandbox payment: %v", err)
	}

	bookingRepo := bookings.NewRepository(pool)
	if _, err := bookingRepo.ConfirmPendingByIDAndCustomerID(ctx, bookingID, customerID); !errors.Is(err, bookings.ErrSandboxPaymentFlowConflict) {
		t.Fatalf("legacy confirm error = %v; want sandbox flow conflict", err)
	}
	if _, err := bookingRepo.UpdatePaymentReference(ctx, bookingID, customerID, "manual-proof"); !errors.Is(err, bookings.ErrSandboxPaymentFlowConflict) {
		t.Fatalf("legacy payment proof error = %v; want sandbox flow conflict", err)
	}

	cancelledBooking, err := bookingRepo.CancelPendingByIDAndCustomerID(ctx, bookingID, customerID)
	if err != nil {
		t.Fatalf("cancel sandbox-backed booking: %v", err)
	}
	if cancelledBooking.Status != "CANCELLED" {
		t.Fatalf("cancelled booking status = %q; want CANCELLED", cancelledBooking.Status)
	}

	var attemptState, commandState string
	var cancellationCount, transitionAuditCount int
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1`, created.Attempt.ID).Scan(&attemptState); err != nil {
		t.Fatalf("read cancelled attempt: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_provider_commands WHERE payment_attempt_id = $1`, created.Attempt.ID).Scan(&commandState); err != nil {
		t.Fatalf("read neutralized command: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_create_cancellations WHERE payment_attempt_id = $1`, created.Attempt.ID).Scan(&cancellationCount); err != nil {
		t.Fatalf("count cancellation tombstones: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM platform_audit_logs
		WHERE entity_id = $1
		  AND action = $2
		  AND metadata->>'from_state' = 'CREATED'
		  AND metadata->>'to_state' = 'CANCELLED'
	`, created.Attempt.ID, audit.ActionPaymentStateTransition).Scan(&transitionAuditCount); err != nil {
		t.Fatalf("count cancellation audit: %v", err)
	}
	if attemptState != string(AttemptStateCancelled) ||
		commandState != string(paymentoutbox.StatePending) ||
		cancellationCount != 1 ||
		transitionAuditCount != 1 {
		t.Fatalf(
			"cancellation facts attempt=%s command=%s tombstones=%d audits=%d; want CANCELLED/PENDING/1/1",
			attemptState,
			commandState,
			cancellationCount,
			transitionAuditCount,
		)
	}
	if _, err := outbox.ClaimNext(ctx, "worker:"+uuid.NewString(), time.Minute); !errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
		t.Fatalf("neutralized command claim error = %v; want no command available", err)
	}

	expiredBookingID, expiredCustomerID := seedOrchestrationBooking(t, ctx, pool)
	expiredAttempt, err := orchestrator.CreatePayment(
		ctx,
		expiredCustomerID,
		expiredBookingID,
		"automatic-expiry-cancel",
		CreateAttemptRequest{RequestedMethod: RequestedMethodCard},
	)
	if err != nil {
		t.Fatalf("create soon-expired sandbox payment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bookings
		SET expires_at = transaction_timestamp() - interval '1 second'
		WHERE id = $1
	`, expiredBookingID); err != nil {
		t.Fatalf("expire sandbox-backed booking: %v", err)
	}
	cancelledCount, err := bookingRepo.CancelExpiredPendingBookings(ctx)
	if err != nil || cancelledCount != 1 {
		t.Fatalf("automatic expired cancellation count=%d error=%v; want 1/nil", cancelledCount, err)
	}
	var expiredBookingStatus, expiredAttemptState string
	var expiredCancellationCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, expiredBookingID).Scan(&expiredBookingStatus); err != nil {
		t.Fatalf("read automatically cancelled booking: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1`, expiredAttempt.Attempt.ID).Scan(&expiredAttemptState); err != nil {
		t.Fatalf("read automatically cancelled attempt: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM payment_create_cancellations
		WHERE payment_attempt_id = $1
		  AND actor_user_id IS NULL
	`, expiredAttempt.Attempt.ID).Scan(&expiredCancellationCount); err != nil {
		t.Fatalf("count automatic cancellation tombstone: %v", err)
	}
	if expiredBookingStatus != "CANCELLED" ||
		expiredAttemptState != string(AttemptStateCancelled) ||
		expiredCancellationCount != 1 {
		t.Fatalf(
			"automatic cancellation booking=%s attempt=%s tombstones=%d; want CANCELLED/CANCELLED/1",
			expiredBookingStatus,
			expiredAttemptState,
			expiredCancellationCount,
		)
	}
	if _, err := outbox.ClaimNext(ctx, "worker:"+uuid.NewString(), time.Minute); !errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
		t.Fatalf("automatically neutralized command claim error = %v; want no command available", err)
	}

	raceBookingID, raceCustomerID := seedOrchestrationBooking(t, ctx, pool)
	raceAttempt, err := orchestrator.CreatePayment(
		ctx,
		raceCustomerID,
		raceBookingID,
		"claim-vs-cancel",
		CreateAttemptRequest{RequestedMethod: RequestedMethodBCAVA},
	)
	if err != nil {
		t.Fatalf("create claim-vs-cancel attempt: %v", err)
	}
	start := make(chan struct{})
	claimResult := make(chan error, 1)
	cancelResult := make(chan error, 1)
	go func() {
		<-start
		_, claimErr := outbox.ClaimNext(ctx, "worker:"+uuid.NewString(), time.Minute)
		claimResult <- claimErr
	}()
	go func() {
		<-start
		_, cancelErr := bookingRepo.CancelPendingByIDAndCustomerID(ctx, raceBookingID, raceCustomerID)
		cancelResult <- cancelErr
	}()
	close(start)
	claimErr := <-claimResult
	cancelErr := <-cancelResult
	if claimErr == nil && cancelErr == nil {
		t.Fatal("claim and local cancellation both succeeded")
	}
	if claimErr == nil {
		if !errors.Is(cancelErr, bookings.ErrSandboxPaymentCancelUnavailable) {
			t.Fatalf("claim winner cancellation error = %v; want dispatch conflict", cancelErr)
		}
	} else {
		if !errors.Is(claimErr, paymentoutbox.ErrNoCommandAvailable) || cancelErr != nil {
			t.Fatalf("cancel winner results claim=%v cancel=%v", claimErr, cancelErr)
		}
	}

	var raceBookingStatus, raceAttemptState, raceCommandState string
	var raceCancellationCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, raceBookingID).Scan(&raceBookingStatus); err != nil {
		t.Fatalf("read claim-vs-cancel booking: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1`, raceAttempt.Attempt.ID).Scan(&raceAttemptState); err != nil {
		t.Fatalf("read claim-vs-cancel attempt: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_provider_commands WHERE payment_attempt_id = $1`, raceAttempt.Attempt.ID).Scan(&raceCommandState); err != nil {
		t.Fatalf("read claim-vs-cancel command: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_create_cancellations WHERE payment_attempt_id = $1`, raceAttempt.Attempt.ID).Scan(&raceCancellationCount); err != nil {
		t.Fatalf("count claim-vs-cancel tombstone: %v", err)
	}
	if claimErr == nil {
		if raceBookingStatus != "PENDING_PAYMENT" ||
			raceAttemptState != string(AttemptStateCreated) ||
			raceCommandState != string(paymentoutbox.StateLeased) ||
			raceCancellationCount != 0 {
			t.Fatalf(
				"claim winner booking=%s attempt=%s command=%s tombstones=%d",
				raceBookingStatus,
				raceAttemptState,
				raceCommandState,
				raceCancellationCount,
			)
		}
	} else if raceBookingStatus != "CANCELLED" ||
		raceAttemptState != string(AttemptStateCancelled) ||
		raceCommandState != string(paymentoutbox.StatePending) ||
		raceCancellationCount != 1 {
		t.Fatalf(
			"cancel winner booking=%s attempt=%s command=%s tombstones=%d",
			raceBookingStatus,
			raceAttemptState,
			raceCommandState,
			raceCancellationCount,
		)
	}
}

func TestSandboxCreateAndLegacyConfirmAreMutuallyExclusiveUnderRace(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	bookingID, customerID := seedOrchestrationBooking(t, ctx, pool)
	orchestrator := NewOrchestrator(
		pool,
		NewRepository(pool),
		paymentoutbox.NewRepository(pool),
		audit.NewPlatformService(audit.NewPlatformRepository()),
		OrchestratorOptions{
			SandboxEnabled: true,
			CreateEnabled:  true,
			AttemptTTL:     time.Hour,
			ReturnOrigin:   "https://demo.example.test",
		},
	)
	bookingRepo := bookings.NewRepository(pool)

	start := make(chan struct{})
	createErr := make(chan error, 1)
	legacyErr := make(chan error, 1)
	go func() {
		<-start
		_, err := orchestrator.CreatePayment(
			ctx,
			customerID,
			strings.ToUpper(bookingID),
			"create-vs-legacy",
			CreateAttemptRequest{RequestedMethod: RequestedMethodBCAVA},
		)
		createErr <- err
	}()
	go func() {
		<-start
		_, err := bookingRepo.ConfirmPendingByIDAndCustomerID(ctx, bookingID, customerID)
		legacyErr <- err
	}()
	close(start)

	createResultErr := <-createErr
	legacyResultErr := <-legacyErr
	if createResultErr == nil && legacyResultErr == nil {
		t.Fatal("sandbox create and legacy confirm both succeeded")
	}
	if createResultErr != nil &&
		!errors.Is(createResultErr, ErrBookingNotPayable) &&
		!errors.Is(createResultErr, ErrBookingNotFound) {
		t.Fatalf("unexpected sandbox race error: %v", createResultErr)
	}
	if legacyResultErr != nil && !errors.Is(legacyResultErr, bookings.ErrSandboxPaymentFlowConflict) {
		t.Fatalf("unexpected legacy race error: %v", legacyResultErr)
	}

	var bookingStatus string
	var attemptCount, commandCount, ownerIncomeCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, bookingID).Scan(&bookingStatus); err != nil {
		t.Fatalf("read raced booking status: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, bookingID).Scan(&attemptCount); err != nil {
		t.Fatalf("count raced attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM payment_provider_commands c
		JOIN payment_attempts pa ON pa.id = c.payment_attempt_id
		WHERE pa.booking_id = $1
	`, bookingID).Scan(&commandCount); err != nil {
		t.Fatalf("count raced commands: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM owner_finance_transactions
		WHERE booking_id = $1
	`, bookingID).Scan(&ownerIncomeCount); err != nil {
		t.Fatalf("count raced owner income: %v", err)
	}

	switch {
	case attemptCount == 1:
		if bookingStatus != "PENDING_PAYMENT" || commandCount != 1 {
			t.Fatalf("sandbox winner left booking=%s attempts=%d commands=%d", bookingStatus, attemptCount, commandCount)
		}
	case attemptCount == 0:
		if bookingStatus != "CONFIRMED" || commandCount != 0 {
			t.Fatalf("legacy winner left booking=%s attempts=%d commands=%d", bookingStatus, attemptCount, commandCount)
		}
	default:
		t.Fatalf("race created %d attempts; want 0 or 1", attemptCount)
	}
	if ownerIncomeCount != 0 {
		t.Fatalf("race inserted %d owner income rows; want 0", ownerIncomeCount)
	}
}

func TestSandboxCreateMapsLegacyOwnerIncomeIsolationToConflict(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	bookingID, customerID := seedOrchestrationBooking(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO owner_finance_transactions (
			owner_id, venue_id, booking_id, created_by_user_id,
			type, source, category, amount, transaction_date, description
		)
		SELECT op.user_id, v.id, b.id, op.user_id,
		       'INCOME', 'BOOKING', 'BOOKING_PAYMENT', b.total_price,
		       CURRENT_DATE, 'legacy fact before sandbox'
		FROM bookings b
		JOIN courts c ON c.id = b.court_id
		JOIN venues v ON v.id = c.venue_id
		JOIN owner_profiles op ON op.id = v.owner_profile_id
		WHERE b.id = $1
	`, bookingID); err != nil {
		t.Fatalf("insert legacy owner income: %v", err)
	}
	orchestrator := NewOrchestrator(
		pool,
		NewRepository(pool),
		paymentoutbox.NewRepository(pool),
		audit.NewPlatformService(audit.NewPlatformRepository()),
		OrchestratorOptions{
			SandboxEnabled: true,
			CreateEnabled:  true,
			AttemptTTL:     time.Hour,
			ReturnOrigin:   "https://demo.example.test",
		},
	)

	_, err := orchestrator.CreatePayment(
		ctx,
		customerID,
		bookingID,
		"legacy-owner-income-conflict",
		CreateAttemptRequest{RequestedMethod: RequestedMethodQRIS},
	)
	if !errors.Is(err, ErrBookingNotPayable) {
		t.Fatalf("create with legacy owner income error = %v; want ErrBookingNotPayable", err)
	}
	assertOrchestrationRowCounts(t, ctx, pool, bookingID, 0, 0, 0, 0)
}

func TestExpiredBookingSweepContinuesAfterPreviouslyDispatchedCommand(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingRepo := bookings.NewRepository(pool)
	orchestrator := NewOrchestrator(
		pool,
		NewRepository(pool),
		outbox,
		audit.NewPlatformService(audit.NewPlatformRepository()),
		OrchestratorOptions{
			SandboxEnabled: true,
			CreateEnabled:  true,
			AttemptTTL:     time.Hour,
			ReturnOrigin:   "https://demo.example.test",
		},
	)

	blockedBookingID, blockedCustomerID := seedOrchestrationBooking(t, ctx, pool)
	blocked, err := orchestrator.CreatePayment(
		ctx,
		blockedCustomerID,
		blockedBookingID,
		"expiry-sweep-dispatched",
		CreateAttemptRequest{RequestedMethod: RequestedMethodBCAVA},
	)
	if err != nil {
		t.Fatalf("create previously dispatched attempt: %v", err)
	}
	claimed, err := outbox.ClaimNext(ctx, "worker:"+uuid.NewString(), time.Minute)
	if err != nil {
		t.Fatalf("claim command before expiry sweep: %v", err)
	}
	if _, err := outbox.MarkRetryable(
		ctx,
		claimed.ID,
		*claimed.LeaseOwner,
		*claimed.LeaseToken,
		"RETRYABLE_TIMEOUT",
		time.Minute,
	); err != nil {
		t.Fatalf("mark command as previously dispatched retryable: %v", err)
	}

	cancellableBookingID, cancellableCustomerID := seedOrchestrationBooking(t, ctx, pool)
	cancellable, err := orchestrator.CreatePayment(
		ctx,
		cancellableCustomerID,
		cancellableBookingID,
		"expiry-sweep-undispatched",
		CreateAttemptRequest{RequestedMethod: RequestedMethodCard},
	)
	if err != nil {
		t.Fatalf("create undispatched attempt: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bookings
		SET expires_at = CASE id
			WHEN $1::uuid THEN transaction_timestamp() - interval '2 minutes'
			WHEN $2::uuid THEN transaction_timestamp() - interval '1 minute'
		END
		WHERE id IN ($1::uuid, $2::uuid)
	`, blockedBookingID, cancellableBookingID); err != nil {
		t.Fatalf("expire sweep candidates: %v", err)
	}

	cancelled, err := bookingRepo.CancelExpiredPendingBookings(ctx)
	if cancelled != 1 || !errors.Is(err, bookings.ErrSandboxPaymentCancelUnavailable) {
		t.Fatalf("expiry sweep result = %d, %v; want 1 and dispatch conflict", cancelled, err)
	}

	var blockedBookingState, blockedAttemptState, cancellableBookingState, cancellableAttemptState string
	var blockedTombstones, cancellableTombstones int
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, blockedBookingID).Scan(&blockedBookingState); err != nil {
		t.Fatalf("read blocked booking: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1`, blocked.Attempt.ID).Scan(&blockedAttemptState); err != nil {
		t.Fatalf("read blocked attempt: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, cancellableBookingID).Scan(&cancellableBookingState); err != nil {
		t.Fatalf("read cancellable booking: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1`, cancellable.Attempt.ID).Scan(&cancellableAttemptState); err != nil {
		t.Fatalf("read cancellable attempt: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_create_cancellations WHERE payment_attempt_id = $1`, blocked.Attempt.ID).Scan(&blockedTombstones); err != nil {
		t.Fatalf("count blocked tombstones: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_create_cancellations WHERE payment_attempt_id = $1`, cancellable.Attempt.ID).Scan(&cancellableTombstones); err != nil {
		t.Fatalf("count cancellable tombstones: %v", err)
	}
	if blockedBookingState != "PENDING_PAYMENT" ||
		blockedAttemptState != string(AttemptStateCreated) ||
		blockedTombstones != 0 ||
		cancellableBookingState != "CANCELLED" ||
		cancellableAttemptState != string(AttemptStateCancelled) ||
		cancellableTombstones != 1 {
		t.Fatalf(
			"sweep facts blocked=%s/%s/%d cancellable=%s/%s/%d",
			blockedBookingState,
			blockedAttemptState,
			blockedTombstones,
			cancellableBookingState,
			cancellableAttemptState,
			cancellableTombstones,
		)
	}
}

type failingPlatformAudit struct {
	err error
}

func (f failingPlatformAudit) Record(context.Context, audit.DBTX, audit.CreatePlatformAuditLogParams) error {
	return f.err
}

type concurrentCreateResult struct {
	result CreatePaymentResult
	err    error
}

func runConcurrentCreates(calls ...func() (CreatePaymentResult, error)) []concurrentCreateResult {
	start := make(chan struct{})
	results := make([]concurrentCreateResult, len(calls))
	var ready sync.WaitGroup
	var complete sync.WaitGroup
	ready.Add(len(calls))
	complete.Add(len(calls))
	for index, call := range calls {
		go func(index int, call func() (CreatePaymentResult, error)) {
			defer complete.Done()
			ready.Done()
			<-start
			results[index].result, results[index].err = call()
		}(index, call)
	}
	ready.Wait()
	close(start)
	complete.Wait()
	return results
}

func seedOrchestrationBooking(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	var customerID string
	if err := pool.QueryRow(ctx, `SELECT customer_id::text FROM bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatalf("read orchestration customer: %v", err)
	}
	return bookingID, customerID
}

func assertOrchestrationRowCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, bookingID string, wantAttempts, wantContracts, wantCommands, wantAudits int) {
	t.Helper()
	var attempts, contracts, commands, audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_attempts WHERE booking_id = $1`, bookingID).Scan(&attempts); err != nil {
		t.Fatalf("count orchestration attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM payment_create_contracts pcc
		JOIN payment_attempts pa ON pa.id = pcc.payment_attempt_id
		WHERE pa.booking_id = $1
	`, bookingID).Scan(&contracts); err != nil {
		t.Fatalf("count orchestration contracts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM payment_provider_commands ppc
		JOIN payment_attempts pa ON pa.id = ppc.payment_attempt_id
		WHERE pa.booking_id = $1
	`, bookingID).Scan(&commands); err != nil {
		t.Fatalf("count orchestration commands: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM platform_audit_logs pal
		JOIN payment_attempts pa ON pa.id = pal.entity_id
		WHERE pa.booking_id = $1
	`, bookingID).Scan(&audits); err != nil {
		t.Fatalf("count orchestration audits: %v", err)
	}
	if attempts != wantAttempts || contracts != wantContracts || commands != wantCommands || audits != wantAudits {
		t.Fatalf(
			"orchestration counts attempts/contracts/commands/audits = %d/%d/%d/%d; want %d/%d/%d/%d",
			attempts, contracts, commands, audits,
			wantAttempts, wantContracts, wantCommands, wantAudits,
		)
	}
}
