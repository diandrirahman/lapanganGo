CREATE TABLE IF NOT EXISTS booking_refund_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    venue_id UUID REFERENCES venues(id) ON DELETE SET NULL,
    reason TEXT NOT NULL,
    status VARCHAR(30) NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED')),
    owner_note TEXT,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ,
    reviewed_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_booking_refund_requests_customer
ON booking_refund_requests(customer_id, requested_at DESC);

CREATE INDEX IF NOT EXISTS idx_booking_refund_requests_owner
ON booking_refund_requests(owner_id, requested_at DESC);

CREATE INDEX IF NOT EXISTS idx_booking_refund_requests_booking
ON booking_refund_requests(booking_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_booking_refund_requests_one_pending_per_booking
ON booking_refund_requests(booking_id)
WHERE status = 'PENDING';

DROP TRIGGER IF EXISTS set_booking_refund_requests_timestamp ON booking_refund_requests;
CREATE TRIGGER set_booking_refund_requests_timestamp
BEFORE UPDATE ON booking_refund_requests
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();
