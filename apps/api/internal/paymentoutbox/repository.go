package paymentoutbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EnqueueTx participates in the caller's transaction. It never begins,
// commits, or rolls back a transaction and never calls a provider.
func (r *Repository) EnqueueTx(ctx context.Context, tx pgx.Tx, params EnqueueParams) (EnqueueResult, error) {
	requestedPayload, err := ValidateEnqueueParams(params)
	if err != nil {
		return EnqueueResult{}, err
	}
	facts, err := loadPaymentAttemptFactsTx(ctx, tx, params.PaymentAttemptID)
	if err != nil {
		return EnqueueResult{}, err
	}
	expectedKey := deterministicIdempotencyKey(
		params.CommandType,
		facts.BookingID,
		facts.AttemptNo,
	)
	canonicalPayloadValue := PaymentCommandPayload{
		AttemptID:       params.PaymentAttemptID,
		AmountRupiah:    facts.AmountRupiah,
		Currency:        facts.Currency,
		RequestedMethod: facts.RequestedMethod,
	}
	canonicalPayload, err := json.Marshal(canonicalPayloadValue)
	if err != nil {
		return EnqueueResult{}, ErrInvalidCommand
	}
	if params.IdempotencyKey != expectedKey ||
		params.RequestHash != facts.RequestHash ||
		params.Payload != canonicalPayloadValue ||
		string(requestedPayload) != string(canonicalPayload) {
		exists, existsErr := commandKeyExistsTx(ctx, tx, params.CommandType, params.IdempotencyKey)
		if existsErr != nil {
			return EnqueueResult{}, existsErr
		}
		if exists {
			return EnqueueResult{}, ErrIdempotencyConflict
		}
		return EnqueueResult{}, ErrInvalidCommand
	}
	params.IdempotencyKey = expectedKey
	params.RequestHash = facts.RequestHash
	payload := canonicalPayload
	var availableAt *time.Time
	if !params.AvailableAt.IsZero() {
		normalizedAvailableAt := params.AvailableAt.UTC()
		availableAt = &normalizedAvailableAt
	}

	var command Command
	err = scanCommand(tx.QueryRow(ctx, `
		INSERT INTO payment_provider_commands (
			command_type, aggregate_type, aggregate_id, payment_attempt_id,
			idempotency_key, request_hash, redacted_payload, available_at
		) VALUES (
			$1, $2, $3::uuid, $4::uuid, $5, $6, $7::jsonb,
			COALESCE($8::timestamptz, transaction_timestamp())
		)
		ON CONFLICT DO NOTHING
		RETURNING id::text, command_type, aggregate_type, aggregate_id::text,
		          payment_attempt_id::text, idempotency_key, request_hash,
		          redacted_payload, state, attempt_count, malformed_response_count, available_at,
		          lease_owner, lease_token::text, lease_expires_at, last_error_code, provider_reference,
		          created_at, updated_at, completed_at
	`, params.CommandType, params.AggregateType, params.AggregateID, params.PaymentAttemptID,
		params.IdempotencyKey, params.RequestHash, payload, availableAt), &command)
	if err == nil {
		return EnqueueResult{Command: command}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return EnqueueResult{}, err
	}

	var samePayload bool
	err = scanCommand(tx.QueryRow(ctx, `
		SELECT id::text, command_type, aggregate_type, aggregate_id::text,
		       payment_attempt_id::text, idempotency_key, request_hash,
		       redacted_payload, state, attempt_count, malformed_response_count, available_at,
		       lease_owner, lease_token::text, lease_expires_at, last_error_code, provider_reference,
		       created_at, updated_at, completed_at,
		       redacted_payload = $3::jsonb
		FROM payment_provider_commands
		WHERE command_type = $1 AND idempotency_key = $2
		FOR UPDATE
	`, params.CommandType, params.IdempotencyKey, payload), &command, &samePayload)
	if errors.Is(err, pgx.ErrNoRows) {
		var aggregateExists bool
		if aggregateErr := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM payment_provider_commands
				WHERE command_type = $1 AND aggregate_type = $2 AND aggregate_id = $3::uuid
			)
		`, params.CommandType, params.AggregateType, params.AggregateID).Scan(&aggregateExists); aggregateErr != nil {
			return EnqueueResult{}, aggregateErr
		}
		if aggregateExists {
			return EnqueueResult{}, ErrIdempotencyConflict
		}
		return EnqueueResult{}, ErrCommandNotFound
	}
	if err != nil {
		return EnqueueResult{}, err
	}
	if command.AggregateType != params.AggregateType ||
		command.AggregateID != params.AggregateID ||
		command.PaymentAttemptID == nil ||
		*command.PaymentAttemptID != params.PaymentAttemptID ||
		command.RequestHash != params.RequestHash ||
		!samePayload {
		return EnqueueResult{}, ErrIdempotencyConflict
	}
	return EnqueueResult{Command: command, Replayed: true}, nil
}

// ClaimNext leases one eligible command and issues a new opaque lease token.
// Every completion operation must present that exact token.
func (r *Repository) ClaimNext(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (Command, error) {
	if !validateLeaseOwner(leaseOwner) || !validateLeaseDuration(leaseDuration) {
		return Command{}, ErrInvalidCommand
	}
	leaseDurationMicroseconds := leaseDuration.Microseconds()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Command{}, err
	}
	defer tx.Rollback(ctx)

	var command Command
	err = scanCommand(tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM payment_provider_commands
			WHERE (state IN ('PENDING', 'RETRYABLE') AND available_at <= transaction_timestamp())
			   OR (state = 'LEASED' AND lease_expires_at <= transaction_timestamp())
			ORDER BY available_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE payment_provider_commands c
		SET state = 'LEASED',
		    attempt_count = c.attempt_count + 1,
		    lease_owner = $1,
		    lease_token = gen_random_uuid(),
		    lease_expires_at = transaction_timestamp() + ($2::bigint * interval '1 microsecond'),
		    last_error_code = CASE WHEN c.state = 'LEASED' THEN 'LEASE_EXPIRED' ELSE c.last_error_code END,
		    updated_at = transaction_timestamp()
		FROM candidate
		WHERE c.id = candidate.id
		RETURNING c.id::text, c.command_type, c.aggregate_type, c.aggregate_id::text,
		          c.payment_attempt_id::text, c.idempotency_key, c.request_hash,
		          c.redacted_payload, c.state, c.attempt_count, c.malformed_response_count, c.available_at,
		          c.lease_owner, c.lease_token::text, c.lease_expires_at, c.last_error_code, c.provider_reference,
		          c.created_at, c.updated_at, c.completed_at
	`, leaseOwner, leaseDurationMicroseconds), &command)
	if errors.Is(err, pgx.ErrNoRows) {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return Command{}, commitErr
		}
		return Command{}, ErrNoCommandAvailable
	}
	if err != nil {
		return Command{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Command{}, err
	}
	return command, nil
}

func (r *Repository) MarkRetryable(ctx context.Context, id, leaseOwner, leaseToken, errorCode string, retryDelay time.Duration) (Command, error) {
	if !validateLeaseOwner(leaseOwner) || !validateLeaseToken(leaseToken) ||
		!validateRetryableErrorCode(errorCode) || !validateRetryDelay(retryDelay) {
		return Command{}, ErrInvalidCommand
	}
	retryDelayMicroseconds := retryDelay.Microseconds()
	if errorCode == "MALFORMED_RESPONSE" {
		return r.finishLease(ctx, id, leaseOwner, leaseToken, `
			state = CASE WHEN malformed_response_count >= 1 THEN 'TERMINAL' ELSE 'RETRYABLE' END,
			available_at = CASE
				WHEN malformed_response_count >= 1 THEN available_at
				ELSE transaction_timestamp() + ($5::bigint * interval '1 microsecond')
			END,
			last_error_code = $4,
			malformed_response_count = malformed_response_count + 1,
			lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
			updated_at = transaction_timestamp(),
			completed_at = CASE WHEN malformed_response_count >= 1 THEN transaction_timestamp() ELSE NULL END
		`, errorCode, retryDelayMicroseconds)
	}
	return r.finishLease(ctx, id, leaseOwner, leaseToken, `
		state = 'RETRYABLE',
		available_at = transaction_timestamp() + ($5::bigint * interval '1 microsecond'),
		last_error_code = $4,
		lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, updated_at = transaction_timestamp(),
		completed_at = NULL
	`, errorCode, retryDelayMicroseconds)
}

func (r *Repository) MarkSucceeded(ctx context.Context, id, leaseOwner, leaseToken, providerReference string) (Command, error) {
	if !validateLeaseOwner(leaseOwner) || !validateLeaseToken(leaseToken) ||
		!validateProviderReference(providerReference) {
		return Command{}, ErrInvalidCommand
	}
	return r.finishLease(ctx, id, leaseOwner, leaseToken, `
		state = 'SUCCEEDED', provider_reference = $4,
		lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, updated_at = transaction_timestamp(),
		completed_at = transaction_timestamp(), last_error_code = NULL
	`, providerReference, nil)
}

type paymentAttemptFacts struct {
	BookingID       string
	AttemptNo       int16
	RequestedMethod string
	Currency        string
	AmountRupiah    int64
	RequestHash     string
}

func loadPaymentAttemptFactsTx(ctx context.Context, tx pgx.Tx, paymentAttemptID string) (paymentAttemptFacts, error) {
	var facts paymentAttemptFacts
	err := tx.QueryRow(ctx, `
		SELECT booking_id::text, attempt_no, requested_method, currency::text,
		       amount_rupiah, request_hash
		FROM payment_attempts
		WHERE id = $1::uuid
		FOR KEY SHARE
	`, paymentAttemptID).Scan(
		&facts.BookingID,
		&facts.AttemptNo,
		&facts.RequestedMethod,
		&facts.Currency,
		&facts.AmountRupiah,
		&facts.RequestHash,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return paymentAttemptFacts{}, ErrInvalidCommand
	}
	if err != nil {
		return paymentAttemptFacts{}, err
	}
	return facts, nil
}

func commandKeyExistsTx(ctx context.Context, tx pgx.Tx, commandType CommandType, idempotencyKey string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM payment_provider_commands
			WHERE command_type = $1 AND idempotency_key = $2
		)
	`, commandType, idempotencyKey).Scan(&exists)
	return exists, err
}

func (r *Repository) MarkTerminal(ctx context.Context, id, leaseOwner, leaseToken, errorCode string) (Command, error) {
	if !validateLeaseOwner(leaseOwner) || !validateLeaseToken(leaseToken) || !validateTerminalErrorCode(errorCode) {
		return Command{}, ErrInvalidCommand
	}
	return r.finishLease(ctx, id, leaseOwner, leaseToken, `
		state = 'TERMINAL', last_error_code = $4,
		lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, updated_at = transaction_timestamp(),
		completed_at = transaction_timestamp()
	`, errorCode, nil)
}

func (r *Repository) finishLease(
	ctx context.Context,
	id, leaseOwner, leaseToken, setClause, value string,
	extraValue any,
) (Command, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Command{}, ErrInvalidCommand
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Command{}, err
	}
	defer tx.Rollback(ctx)

	var command Command
	query := `
		UPDATE payment_provider_commands
		SET ` + setClause + `
		WHERE id = $1::uuid AND state = 'LEASED' AND lease_owner = $2
		  AND lease_token = $3::uuid
		  AND lease_expires_at > transaction_timestamp()
		RETURNING id::text, command_type, aggregate_type, aggregate_id::text,
		          payment_attempt_id::text, idempotency_key, request_hash,
		          redacted_payload, state, attempt_count, malformed_response_count, available_at,
		          lease_owner, lease_token::text, lease_expires_at, last_error_code, provider_reference,
		          created_at, updated_at, completed_at
	`
	args := []any{id, leaseOwner, leaseToken, value}
	if extraValue != nil {
		args = append(args, extraValue)
	}
	if err := scanCommand(tx.QueryRow(ctx, query, args...), &command); errors.Is(err, pgx.ErrNoRows) {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return Command{}, commitErr
		}
		return Command{}, ErrLeaseConflict
	} else if err != nil {
		return Command{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Command{}, err
	}
	return command, nil
}

func scanCommand(row pgx.Row, destinations ...any) error {
	var command Command
	var payload []byte
	var samePayload bool
	scanArgs := []any{
		&command.ID, &command.CommandType, &command.AggregateType, &command.AggregateID,
		&command.PaymentAttemptID, &command.IdempotencyKey, &command.RequestHash,
		&payload, &command.State, &command.AttemptCount, &command.MalformedResponseCount, &command.AvailableAt,
		&command.LeaseOwner, &command.LeaseToken, &command.LeaseExpiresAt, &command.LastErrorCode,
		&command.ProviderReference, &command.CreatedAt, &command.UpdatedAt, &command.CompletedAt,
	}
	if len(destinations) == 2 {
		scanArgs = append(scanArgs, &samePayload)
	}
	if err := row.Scan(scanArgs...); err != nil {
		return err
	}
	command.Payload = append(command.Payload[:0], payload...)
	if len(destinations) == 1 {
		if target, ok := destinations[0].(*Command); ok {
			*target = command
			return nil
		}
	}
	if len(destinations) == 2 {
		commandTarget, commandOK := destinations[0].(*Command)
		payloadTarget, payloadOK := destinations[1].(*bool)
		if commandOK && payloadOK {
			*commandTarget = command
			*payloadTarget = samePayload
			return nil
		}
	}
	return errors.New("invalid command scan destination")
}
