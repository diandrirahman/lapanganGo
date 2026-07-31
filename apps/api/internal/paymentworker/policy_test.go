package paymentworker

import (
	"testing"
	"time"
)

func TestRetryPolicyCapsExponentialDelay(t *testing.T) {
	policy := RetryPolicy{InitialDelay: time.Second, MaxDelay: 4 * time.Second}
	for _, tc := range []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: time.Second},
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 3, want: 4 * time.Second},
		{attempt: 8, want: 4 * time.Second},
	} {
		if got := policy.delay(tc.attempt); got != tc.want {
			t.Errorf("delay(%d) = %s; want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestRetryPolicyBoundsRetryAfter(t *testing.T) {
	policy := RetryPolicy{InitialDelay: 2 * time.Second, MaxDelay: 10 * time.Second}
	if got := policy.delayWithRetryAfter(1, -time.Second); got != 2*time.Second {
		t.Fatalf("negative retry-after = %s; want initial delay", got)
	}
	if got := policy.delayWithRetryAfter(1, 5*time.Second); got != 5*time.Second {
		t.Fatalf("retry-after = %s; want 5s", got)
	}
	if got := policy.delayWithRetryAfter(1, 90*time.Second); got != 10*time.Second {
		t.Fatalf("oversized retry-after = %s; want cap", got)
	}
}

func TestRetryPolicyJitterIsDeterministicAndBounded(t *testing.T) {
	policy := DefaultRetryPolicy()
	first := policy.Delay("payment:inquiry:a", 3, 0)
	second := policy.Delay("payment:inquiry:a", 3, 0)
	if first != second {
		t.Fatalf("same key/attempt jitter differs: %s vs %s", first, second)
	}
	if first < 8*time.Second || first > 9*time.Second {
		t.Fatalf("jitter escaped expected 0-10%% window: %s", first)
	}
	if got := policy.Delay("payment:inquiry:a", 1, 59*time.Second); got < 59*time.Second || got > 60*time.Second {
		t.Fatalf("retry-after was not a lower bound within cap: %s", got)
	}
}

func TestRetryPolicyAlwaysProducesOutboxCompatibleMicroseconds(t *testing.T) {
	policy := RetryPolicy{InitialDelay: time.Millisecond, MaxDelay: time.Second, Jitter: deterministicJitter}
	got := policy.Delay("payment:inquiry:a", 1, 0)
	if got%time.Microsecond != 0 {
		t.Fatalf("retry delay %s is not microsecond aligned", got)
	}
	if got < time.Millisecond || got > 1100*time.Microsecond {
		t.Fatalf("retry delay %s escaped expected jitter bounds", got)
	}

	retryAfter := 1500*time.Microsecond + 300*time.Nanosecond
	got = policy.Delay("payment:inquiry:b", 1, retryAfter)
	if got%time.Microsecond != 0 || got < retryAfter || got > policy.MaxDelay {
		t.Fatalf("retry-after normalized delay = %s; retry-after=%s max=%s", got, retryAfter, policy.MaxDelay)
	}

	capped := RetryPolicy{
		InitialDelay: 999*time.Microsecond + 900*time.Nanosecond,
		MaxDelay:     time.Millisecond + 500*time.Nanosecond,
		Jitter:       func(string, int, time.Duration) time.Duration { return 2 * time.Millisecond },
	}.Delay("payment:inquiry:c", 1, 0)
	if capped != time.Millisecond || capped%time.Microsecond != 0 {
		t.Fatalf("normalized cap = %s; want 1ms", capped)
	}
}
