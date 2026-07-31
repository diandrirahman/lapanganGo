package payments

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"lapangango-api/internal/paymentflow"
	"lapangango-api/internal/paymentoutbox"
)

func TestPaymentProviderOutboxAtomicReplayConflictAndLeaseRecovery(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-atomic")

	params := outboxParams(attempt)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin rollback transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE payment_attempts SET state = 'PENDING', updated_at = transaction_timestamp() WHERE id = $1`, attempt.ID); err != nil {
		t.Fatalf("update domain state in rollback transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, params); err != nil {
		t.Fatalf("enqueue in rollback transaction: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback domain and outbox transaction: %v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1`, attempt.ID).Scan(&state); err != nil {
		t.Fatalf("read rolled back attempt: %v", err)
	}
	if state != string(AttemptStateCreated) {
		t.Fatalf("attempt state after rollback = %q; want CREATED", state)
	}
	var commandCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_provider_commands WHERE idempotency_key = $1`, params.IdempotencyKey).Scan(&commandCount); err != nil {
		t.Fatalf("count rolled back command: %v", err)
	}
	if commandCount != 0 {
		t.Fatalf("rolled back command count = %d; want 0", commandCount)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*paymentoutbox.EnqueueParams)
	}{
		{name: "amount mismatch", mutate: func(p *paymentoutbox.EnqueueParams) { p.Payload.AmountRupiah++ }},
		{name: "method mismatch", mutate: func(p *paymentoutbox.EnqueueParams) { p.Payload.RequestedMethod = "CARD" }},
		{name: "hash mismatch", mutate: func(p *paymentoutbox.EnqueueParams) { p.RequestHash = strings.Repeat("f", 64) }},
		{name: "non-deterministic key", mutate: func(p *paymentoutbox.EnqueueParams) {
			p.IdempotencyKey = "payment:create:" + attempt.BookingID + ":999"
		}},
	} {
		t.Run("reject first enqueue "+tc.name, func(t *testing.T) {
			invalid := params
			tc.mutate(&invalid)
			invalidTx, beginErr := pool.Begin(ctx)
			if beginErr != nil {
				t.Fatalf("begin invalid command transaction: %v", beginErr)
			}
			defer invalidTx.Rollback(ctx)
			if _, enqueueErr := outbox.EnqueueTx(ctx, invalidTx, invalid); !errors.Is(enqueueErr, paymentoutbox.ErrInvalidCommand) {
				t.Fatalf("invalid command error = %v; want ErrInvalidCommand", enqueueErr)
			}
		})
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin commit transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE payment_attempts SET state = 'PENDING', updated_at = transaction_timestamp() WHERE id = $1`, attempt.ID); err != nil {
		t.Fatalf("update domain state in commit transaction: %v", err)
	}
	created, err := outbox.EnqueueTx(ctx, tx, params)
	if err != nil {
		t.Fatalf("enqueue committed command: %v", err)
	}
	var databaseTransactionTime time.Time
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&databaseTransactionTime); err != nil {
		t.Fatalf("read enqueue database transaction time: %v", err)
	}
	if !created.Command.AvailableAt.Equal(databaseTransactionTime) {
		t.Fatalf(
			"default available_at = %s; want database transaction time %s",
			created.Command.AvailableAt,
			databaseTransactionTime,
		)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}
	if created.Replayed || created.Command.State != paymentoutbox.StatePending {
		t.Fatalf("unexpected first command result: %#v", created)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM payment_attempts WHERE id = $1`, attempt.ID).Scan(&state); err != nil {
		t.Fatalf("read committed attempt: %v", err)
	}
	if state != string(AttemptStatePending) {
		t.Fatalf("attempt state after commit = %q; want PENDING", state)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin replay transaction: %v", err)
	}
	replay, err := outbox.EnqueueTx(ctx, tx, params)
	if err != nil || !replay.Replayed || replay.Command.ID != created.Command.ID {
		t.Fatalf("same payload replay = %#v, %v", replay, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit replay transaction: %v", err)
	}

	conflictParams := params
	conflictParams.Payload.AmountRupiah++
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin conflict transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, conflictParams); !errors.Is(err, paymentoutbox.ErrIdempotencyConflict) {
		t.Fatalf("different payload conflict = %v; want ErrIdempotencyConflict", err)
	}
	_ = tx.Rollback(ctx)

	wrongKeyParams := params
	wrongKeyParams.IdempotencyKey = "payment:create:" + attempt.BookingID + ":999"
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin duplicate aggregate transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, wrongKeyParams); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
		t.Fatalf("different key for same aggregate = %v; want ErrInvalidCommand", err)
	}
	_ = tx.Rollback(ctx)
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM payment_provider_commands
		WHERE command_type = 'PAYMENT_CREATE' AND aggregate_id = $1
	`, attempt.ID).Scan(&commandCount); err != nil {
		t.Fatalf("count aggregate commands: %v", err)
	}
	if commandCount != 1 {
		t.Fatalf("payment create commands for aggregate = %d; want 1", commandCount)
	}

	restartOwner := "worker:" + uuid.NewString()
	claimed, err := outbox.ClaimNext(ctx, restartOwner, time.Minute)
	if err != nil || claimed.ID != created.Command.ID || claimed.AttemptCount != 1 {
		t.Fatalf("claim result = %#v, %v", claimed, err)
	}
	if claimed.LeaseToken == nil {
		t.Fatal("claim did not issue a lease token")
	}
	if _, err := outbox.MarkRetryable(ctx, claimed.ID, *claimed.LeaseOwner, *claimed.LeaseToken, "AUTHENTICATION_FAILED", 0); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
		t.Fatalf("terminal error accepted as retryable: %v", err)
	}
	if _, err := outbox.MarkTerminal(ctx, claimed.ID, *claimed.LeaseOwner, *claimed.LeaseToken, "RETRYABLE_TIMEOUT"); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
		t.Fatalf("retryable error accepted as terminal: %v", err)
	}
	if _, err := outbox.MarkRetryable(ctx, claimed.ID, *claimed.LeaseOwner, *claimed.LeaseToken, "RETRYABLE_TIMEOUT", 0); err != nil {
		t.Fatalf("mark retryable: %v", err)
	}
	claimedAgain, err := outbox.ClaimNext(ctx, restartOwner, 50*time.Millisecond)
	if err != nil || claimedAgain.ID != created.Command.ID || claimedAgain.AttemptCount != 2 {
		t.Fatalf("retry claim result = %#v, %v", claimedAgain, err)
	}
	if claimedAgain.LeaseToken == nil || *claimedAgain.LeaseToken == *claimed.LeaseToken {
		t.Fatal("retry claim did not rotate the lease token")
	}
	staleToken := *claimedAgain.LeaseToken
	time.Sleep(75 * time.Millisecond)
	reclaimed, err := outbox.ClaimNext(ctx, restartOwner, time.Minute)
	if err != nil || reclaimed.ID != created.Command.ID || reclaimed.AttemptCount != 3 {
		t.Fatalf("expired lease reclaim result = %#v, %v", reclaimed, err)
	}
	if reclaimed.LeaseToken == nil || *reclaimed.LeaseToken == staleToken {
		t.Fatal("expired lease reclaim did not rotate the lease token")
	}
	safeProviderReference, err := paymentoutbox.DigestProviderReference("ps_1234abcd")
	if err != nil {
		t.Fatalf("digest provider reference: %v", err)
	}
	if _, err := outbox.MarkSucceeded(ctx, reclaimed.ID, restartOwner, staleToken, safeProviderReference); !errors.Is(err, paymentoutbox.ErrLeaseConflict) {
		t.Fatalf("same-owner stale lease completion = %v; want ErrLeaseConflict", err)
	}
	for _, unsafeReference := range []string{
		"4111111111111111",
		"ref-4111-1111-1111-1111",
		"ref_1234567890",
		"sk_test_abc123",
		"xnd_development_secret",
		"https://provider.example/reference",
	} {
		if _, err := outbox.MarkSucceeded(ctx, reclaimed.ID, restartOwner, *reclaimed.LeaseToken, unsafeReference); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
			t.Fatalf("unsafe provider reference %q = %v; want ErrInvalidCommand", unsafeReference, err)
		}
	}
	if _, err := outbox.MarkSucceeded(ctx, reclaimed.ID, restartOwner, *reclaimed.LeaseToken, safeProviderReference); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
}

func TestPaymentProviderOutboxMalformedResponseRetriesOnlyOnce(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-malformed")
	params := outboxParams(attempt)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin command transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, params); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}

	workerOwner := "worker:" + uuid.NewString()
	first, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || first.LeaseToken == nil {
		t.Fatalf("first malformed claim = %#v, %v", first, err)
	}
	firstRetry, err := outbox.MarkRetryable(ctx, first.ID, *first.LeaseOwner, *first.LeaseToken, "MALFORMED_RESPONSE", 0)
	if err != nil {
		t.Fatalf("first malformed response should be retryable: %v", err)
	}
	if firstRetry.State != paymentoutbox.StateRetryable || firstRetry.MalformedResponseCount != 1 {
		t.Fatalf("first malformed response result = %#v; want retryable with malformed count 1", firstRetry)
	}
	if !firstRetry.AvailableAt.Equal(firstRetry.UpdatedAt) {
		t.Fatalf(
			"immediate malformed retry available_at = %s; want database update time %s",
			firstRetry.AvailableAt,
			firstRetry.UpdatedAt,
		)
	}
	second, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || second.LeaseToken == nil {
		t.Fatalf("second malformed claim = %#v, %v", second, err)
	}
	terminal, err := outbox.MarkRetryable(ctx, second.ID, *second.LeaseOwner, *second.LeaseToken, "MALFORMED_RESPONSE", 0)
	if err != nil {
		t.Fatalf("second malformed response terminalization: %v", err)
	}
	if terminal.State != paymentoutbox.StateTerminal || terminal.CompletedAt == nil ||
		terminal.LastErrorCode == nil || *terminal.LastErrorCode != "MALFORMED_RESPONSE" ||
		terminal.MalformedResponseCount != 2 {
		t.Fatalf("second malformed response result = %#v; want terminal malformed response", terminal)
	}
	if _, err := outbox.ClaimNext(ctx, workerOwner, time.Minute); !errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
		t.Fatalf("terminal malformed command was claimable: %v", err)
	}
}

func TestPaymentProviderOutboxLeaseDurationUsesDatabasePrecision(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-lease-duration")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin command transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, outboxParams(attempt)); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}

	workerOwner := "worker:" + uuid.NewString()
	for _, invalidDuration := range []time.Duration{
		-time.Nanosecond,
		0,
		time.Nanosecond,
		time.Microsecond - time.Nanosecond,
		time.Microsecond + time.Nanosecond,
		24*time.Hour - time.Nanosecond,
		24*time.Hour + time.Microsecond,
	} {
		if _, err := outbox.ClaimNext(ctx, workerOwner, invalidDuration); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
			t.Fatalf("invalid lease duration %s = %v; want ErrInvalidCommand", invalidDuration, err)
		}
	}

	const leaseDuration = time.Microsecond
	claimed, err := outbox.ClaimNext(ctx, workerOwner, leaseDuration)
	if err != nil || claimed.LeaseExpiresAt == nil {
		t.Fatalf("minimum lease claim = %#v, %v", claimed, err)
	}
	if claimed.LeaseExpiresAt.Sub(claimed.UpdatedAt) != leaseDuration {
		t.Fatalf(
			"lease duration = %s; want exact PostgreSQL duration %s",
			claimed.LeaseExpiresAt.Sub(claimed.UpdatedAt),
			leaseDuration,
		)
	}
}

func TestPaymentProviderOutboxLeaseStartsAfterBookingFlowLock(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-lock-wait")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin command transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, outboxParams(attempt)); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin booking-flow blocker: %v", err)
	}
	defer blocker.Rollback(ctx)
	if err := paymentflow.LockBooking(ctx, blocker, bookingID); err != nil {
		t.Fatalf("lock booking flow: %v", err)
	}

	type claimResult struct {
		command paymentoutbox.Command
		err     error
	}
	result := make(chan claimResult, 1)
	go func() {
		command, claimErr := outbox.ClaimNext(ctx, "worker:"+uuid.NewString(), 50*time.Millisecond)
		result <- claimResult{command: command, err: claimErr}
	}()
	time.Sleep(100 * time.Millisecond)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release booking-flow blocker: %v", err)
	}

	claimed := <-result
	if claimed.err != nil || claimed.command.LeaseExpiresAt == nil {
		t.Fatalf("claim after booking-flow wait = %#v, %v", claimed.command, claimed.err)
	}
	var databaseNow time.Time
	if err := pool.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&databaseNow); err != nil {
		t.Fatalf("read database clock after claim: %v", err)
	}
	if !claimed.command.LeaseExpiresAt.After(databaseNow) {
		t.Fatalf(
			"lease expired during advisory wait: expires=%s database_now=%s",
			claimed.command.LeaseExpiresAt,
			databaseNow,
		)
	}
	if claimed.command.LeaseExpiresAt.Sub(claimed.command.UpdatedAt) != 50*time.Millisecond {
		t.Fatalf(
			"lease duration after advisory wait = %s; want 50ms",
			claimed.command.LeaseExpiresAt.Sub(claimed.command.UpdatedAt),
		)
	}
}

func TestPaymentProviderOutboxRetryDelayUsesDatabaseClock(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-retry-delay")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin command transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, outboxParams(attempt)); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}

	workerOwner := "worker:" + uuid.NewString()
	claimed, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || claimed.LeaseOwner == nil || claimed.LeaseToken == nil {
		t.Fatalf("retry-delay claim = %#v, %v", claimed, err)
	}
	for _, invalidDelay := range []time.Duration{
		-time.Nanosecond,
		time.Nanosecond,
		24*time.Hour - time.Nanosecond,
		24*time.Hour + time.Nanosecond,
	} {
		if _, err := outbox.MarkRetryable(
			ctx,
			claimed.ID,
			*claimed.LeaseOwner,
			*claimed.LeaseToken,
			"RETRYABLE_TIMEOUT",
			invalidDelay,
		); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
			t.Fatalf("invalid retry delay %s = %v; want ErrInvalidCommand", invalidDelay, err)
		}
	}

	const retryDelay = 250 * time.Millisecond
	retryable, err := outbox.MarkRetryable(
		ctx,
		claimed.ID,
		*claimed.LeaseOwner,
		*claimed.LeaseToken,
		"RETRYABLE_TIMEOUT",
		retryDelay,
	)
	if err != nil {
		t.Fatalf("mark delayed retryable: %v", err)
	}
	if retryable.State != paymentoutbox.StateRetryable ||
		retryable.AvailableAt.Sub(retryable.UpdatedAt) != retryDelay {
		t.Fatalf(
			"delayed retry = %#v; want available_at exactly %s after database update time",
			retryable,
			retryDelay,
		)
	}
	if _, err := outbox.ClaimNext(ctx, workerOwner, time.Minute); !errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
		t.Fatalf("delayed retry was claimable too early: %v", err)
	}
	time.Sleep(retryDelay + 100*time.Millisecond)
	reclaimed, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || reclaimed.ID != retryable.ID || reclaimed.AttemptCount != 2 {
		t.Fatalf("delayed retry claim = %#v, %v", reclaimed, err)
	}
}

func TestPaymentProviderOutboxMalformedRetryBudgetIgnoresPriorClaims(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-mixed-malformed")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin command transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, outboxParams(attempt)); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}

	workerOwner := "worker:" + uuid.NewString()
	first, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || first.LeaseToken == nil {
		t.Fatalf("first mixed claim = %#v, %v", first, err)
	}
	if _, err := outbox.MarkRetryable(ctx, first.ID, *first.LeaseOwner, *first.LeaseToken, "RETRYABLE_TIMEOUT", 0); err != nil {
		t.Fatalf("mark initial timeout retryable: %v", err)
	}
	second, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || second.LeaseToken == nil || second.AttemptCount != 2 {
		t.Fatalf("second mixed claim = %#v, %v", second, err)
	}
	firstMalformed, err := outbox.MarkRetryable(ctx, second.ID, *second.LeaseOwner, *second.LeaseToken, "MALFORMED_RESPONSE", 0)
	if err != nil {
		t.Fatalf("first malformed after timeout: %v", err)
	}
	if firstMalformed.State != paymentoutbox.StateRetryable ||
		firstMalformed.AttemptCount != 2 ||
		firstMalformed.MalformedResponseCount != 1 {
		t.Fatalf("first malformed after timeout = %#v; want retryable with independent malformed count", firstMalformed)
	}
	third, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || third.LeaseToken == nil || third.AttemptCount != 3 {
		t.Fatalf("third mixed claim = %#v, %v", third, err)
	}
	secondMalformed, err := outbox.MarkRetryable(ctx, third.ID, *third.LeaseOwner, *third.LeaseToken, "MALFORMED_RESPONSE", 0)
	if err != nil {
		t.Fatalf("second malformed after timeout: %v", err)
	}
	if secondMalformed.State != paymentoutbox.StateTerminal ||
		secondMalformed.MalformedResponseCount != 2 ||
		secondMalformed.CompletedAt == nil {
		t.Fatalf("second malformed after timeout = %#v; want terminal with malformed count 2", secondMalformed)
	}
}

func TestPaymentProviderOutboxMarkTerminalCompletesAndCannotReplay(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-terminal")
	params := outboxParams(attempt)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin command transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, params); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}

	workerOwner := "worker:" + uuid.NewString()
	claimed, err := outbox.ClaimNext(ctx, workerOwner, time.Minute)
	if err != nil || claimed.LeaseOwner == nil || claimed.LeaseToken == nil {
		t.Fatalf("terminal claim = %#v, %v", claimed, err)
	}
	terminal, err := outbox.MarkTerminal(
		ctx,
		claimed.ID,
		*claimed.LeaseOwner,
		*claimed.LeaseToken,
		"AUTHENTICATION_FAILED",
	)
	if err != nil {
		t.Fatalf("mark terminal: %v", err)
	}
	if terminal.State != paymentoutbox.StateTerminal ||
		terminal.CompletedAt == nil ||
		terminal.LastErrorCode == nil ||
		*terminal.LastErrorCode != "AUTHENTICATION_FAILED" ||
		terminal.LeaseOwner != nil ||
		terminal.LeaseToken != nil ||
		terminal.LeaseExpiresAt != nil ||
		terminal.ProviderReference != nil {
		t.Fatalf("terminal command = %#v; want completed terminal state with cleared lease", terminal)
	}
	if _, err := outbox.MarkTerminal(
		ctx,
		claimed.ID,
		*claimed.LeaseOwner,
		*claimed.LeaseToken,
		"AUTHENTICATION_FAILED",
	); !errors.Is(err, paymentoutbox.ErrLeaseConflict) {
		t.Fatalf("terminal command replay = %v; want ErrLeaseConflict", err)
	}
	if _, err := outbox.ClaimNext(ctx, workerOwner, time.Minute); !errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
		t.Fatalf("terminal command was claimable: %v", err)
	}
}

func TestPaymentProviderOutboxConcurrentClaimDoesNotDuplicate(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:create:outbox-concurrent")
	params := outboxParams(attempt)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin command transaction: %v", err)
	}
	if _, err := outbox.EnqueueTx(ctx, tx, params); err != nil {
		t.Fatalf("enqueue command: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit command: %v", err)
	}

	results := make(chan paymentoutbox.Command, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			command, err := outbox.ClaimNext(ctx, "worker:"+uuid.NewString(), time.Minute)
			if err != nil {
				errs <- err
				return
			}
			results <- command
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var claimed int
	for range results {
		claimed++
	}
	var noCommand int
	for err := range errs {
		if errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
			noCommand++
		} else {
			t.Fatalf("concurrent claim error: %v", err)
		}
	}
	if claimed != 1 || noCommand != 1 {
		t.Fatalf("concurrent claim counts = claimed:%d no-command:%d; want 1/1", claimed, noCommand)
	}
}

func TestPaymentProviderOutboxClaimsInquiryAfterBookingCancellation(t *testing.T) {
	ctx, pool := openPaymentTestDB(t)
	outbox := paymentoutbox.NewRepository(pool)
	bookingID := seedRepositoryBooking(t, ctx, pool, true)
	attempt := createOutboxTestAttempt(t, ctx, pool, bookingID, "payment:inquiry:cancelled-booking")

	if _, err := pool.Exec(ctx, `
		UPDATE payment_attempts
		SET state = 'PENDING', updated_at = transaction_timestamp()
		WHERE id = $1
	`, attempt.ID); err != nil {
		t.Fatalf("mark uncertain payment attempt pending: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bookings
		SET status = 'CANCELLED', updated_at = transaction_timestamp()
		WHERE id = $1
	`, bookingID); err != nil {
		t.Fatalf("cancel booking before inquiry: %v", err)
	}

	params := outboxParams(attempt)
	params.CommandType = paymentoutbox.CommandPaymentInquiry
	params.IdempotencyKey = paymentoutbox.DeterministicInquiryKey(attempt.ID)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin inquiry enqueue: %v", err)
	}
	enqueued, err := outbox.EnqueueTx(ctx, tx, params)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("enqueue reserved payment inquiry: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit inquiry enqueue: %v", err)
	}

	claimed, err := outbox.ClaimNext(ctx, "worker:"+uuid.NewString(), time.Minute)
	if err != nil {
		t.Fatalf("claim inquiry after booking cancellation: %v", err)
	}
	if claimed.ID != enqueued.Command.ID ||
		claimed.CommandType != paymentoutbox.CommandPaymentInquiry ||
		claimed.State != paymentoutbox.StateLeased {
		t.Fatalf("claimed command = %#v; want leased inquiry %s", claimed, enqueued.Command.ID)
	}
}

func outboxParams(attempt PaymentAttempt) paymentoutbox.EnqueueParams {
	return paymentoutbox.EnqueueParams{
		CommandType:      paymentoutbox.CommandPaymentCreate,
		AggregateType:    paymentoutbox.AggregatePaymentAttempt,
		AggregateID:      attempt.ID,
		PaymentAttemptID: attempt.ID,
		IdempotencyKey:   "payment:create:" + attempt.BookingID + ":" + strconv.Itoa(int(attempt.AttemptNo)),
		RequestHash:      attempt.RequestHash,
		Payload: paymentoutbox.PaymentCommandPayload{
			AttemptID:       attempt.ID,
			AmountRupiah:    attempt.AmountRupiah,
			Currency:        string(attempt.Currency),
			RequestedMethod: string(attempt.RequestedMethod),
		},
	}
}

func createOutboxTestAttempt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, bookingID, reference string) PaymentAttempt {
	t.Helper()
	attempt, err := NewRepository(pool).CreateOrReplayAttempt(ctx, validCreateParams(bookingID, reference))
	if err != nil {
		t.Fatalf("create outbox test attempt: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO payment_create_contracts (
			payment_attempt_id, request_hash, requested_expires_at,
			success_return_url, cancel_return_url
		) VALUES ($1, $2, $3, $4, $5)
	`, attempt.ID, attempt.RequestHash, attempt.ExpiresAt,
		"https://demo.example.test/payments/return/"+attempt.LocalReference+"/success",
		"https://demo.example.test/payments/return/"+attempt.LocalReference+"/cancel"); err != nil {
		t.Fatalf("create outbox test contract: %v", err)
	}
	return attempt
}
