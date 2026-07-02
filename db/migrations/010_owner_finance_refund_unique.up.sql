CREATE UNIQUE INDEX IF NOT EXISTS idx_owner_finance_transactions_unique_refund_booking
ON owner_finance_transactions(booking_id)
WHERE source = 'REFUND' AND booking_id IS NOT NULL;
