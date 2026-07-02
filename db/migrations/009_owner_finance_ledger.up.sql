CREATE TABLE IF NOT EXISTS owner_finance_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    venue_id UUID REFERENCES venues(id) ON DELETE SET NULL,
    booking_id UUID REFERENCES bookings(id) ON DELETE SET NULL,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    type VARCHAR(50) NOT NULL CHECK (type IN ('INCOME', 'EXPENSE')),
    source VARCHAR(50) NOT NULL CHECK (source IN ('BOOKING', 'MANUAL', 'REFUND', 'PAYROLL', 'MAINTENANCE', 'OTHER')),
    category VARCHAR(255) NOT NULL,
    amount NUMERIC(14,2) NOT NULL CHECK (amount > 0),
    transaction_date DATE NOT NULL,
    payment_method VARCHAR(255),
    description TEXT,
    attachment_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for fast querying based on common filters
CREATE INDEX IF NOT EXISTS idx_owner_finance_transactions_owner_date ON owner_finance_transactions(owner_id, transaction_date);
CREATE INDEX IF NOT EXISTS idx_owner_finance_transactions_owner_venue ON owner_finance_transactions(owner_id, venue_id);
CREATE INDEX IF NOT EXISTS idx_owner_finance_transactions_owner_type ON owner_finance_transactions(owner_id, type);

-- Unique partial index for booking_id to prevent double-counting of BOOKING source transactions
CREATE UNIQUE INDEX IF NOT EXISTS idx_owner_finance_transactions_unique_booking 
ON owner_finance_transactions(booking_id) 
WHERE source = 'BOOKING' AND booking_id IS NOT NULL;

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update updated_at
DROP TRIGGER IF EXISTS update_owner_finance_transactions_updated_at ON owner_finance_transactions;
CREATE TRIGGER update_owner_finance_transactions_updated_at
    BEFORE UPDATE ON owner_finance_transactions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
