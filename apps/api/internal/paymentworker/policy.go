package paymentworker

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"
	"time"
)

// RetryPolicy keeps provider-neutral test recovery deterministic and bounded.
// The real provider adapter is intentionally not part of this package yet.
type RetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Jitter       JitterFunc
}

// JitterFunc is deterministic when keyed by command identity. It must never
// return a duration below base unless the caller explicitly chose to do so.
type JitterFunc func(key string, attempt int, base time.Duration) time.Duration

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{InitialDelay: 2 * time.Second, MaxDelay: 60 * time.Second, Jitter: deterministicJitter}
}

func (p RetryPolicy) delay(attemptCount int) time.Duration {
	return p.baseDelay(attemptCount, 0)
}

func (p RetryPolicy) delayWithRetryAfter(attemptCount int, retryAfter time.Duration) time.Duration {
	return p.baseDelay(attemptCount, retryAfter)
}

// Delay calculates bounded exponential retry delay with deterministic jitter.
// Retry-After is a lower bound and the configured maximum remains a hard cap.
func (p RetryPolicy) Delay(key string, attemptCount int, retryAfter time.Duration) time.Duration {
	base := p.baseDelay(attemptCount, retryAfter)
	if p.Jitter == nil {
		return normalizeRetryDelay(base, p.maxDelay())
	}
	delayed := p.Jitter(key, attemptCount, base)
	if delayed < base {
		delayed = base
	}
	if delayed > p.maxDelay() {
		delayed = p.maxDelay()
	}
	return normalizeRetryDelay(delayed, p.maxDelay())
}

func (p RetryPolicy) baseDelay(attemptCount int, retryAfter time.Duration) time.Duration {
	if p.InitialDelay <= 0 {
		p.InitialDelay = 2 * time.Second
	}
	if p.MaxDelay < p.InitialDelay {
		p.MaxDelay = 60 * time.Second
	}
	if attemptCount < 1 {
		attemptCount = 1
	}
	delay := p.InitialDelay
	for i := 1; i < attemptCount; i++ {
		if delay >= p.MaxDelay/2 {
			return p.MaxDelay
		}
		delay *= 2
	}
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	if retryAfter > delay {
		if retryAfter > p.MaxDelay {
			return p.MaxDelay
		}
		return retryAfter
	}
	return delay
}

func (p RetryPolicy) maxDelay() time.Duration {
	if p.MaxDelay < p.InitialDelay || p.MaxDelay <= 0 {
		return 60 * time.Second
	}
	return p.MaxDelay
}

func deterministicJitter(key string, attempt int, base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	sum := sha256.Sum256([]byte(key + ":" + strconv.Itoa(attempt)))
	// Add at most ten percent. The hash makes retries stable across worker
	// restarts while still spreading different command keys.
	spread := binary.BigEndian.Uint16(sum[:2]) % 1001
	extra := time.Duration((int64(base) * int64(spread)) / 10000)
	return base + extra
}

func normalizeRetryDelay(value, max time.Duration) time.Duration {
	if value <= 0 || max <= 0 {
		return 0
	}
	max = max.Truncate(time.Microsecond)
	if max <= 0 {
		return 0
	}
	if value >= max {
		return max
	}
	remainder := value % time.Microsecond
	if remainder != 0 {
		value += time.Microsecond - remainder
	}
	if value > max {
		return max
	}
	return value
}
