BEGIN;

DROP TRIGGER IF EXISTS prevent_mutation_booking_fee_snapshots ON booking_fee_snapshots;
DROP TRIGGER IF EXISTS prevent_mutation_platform_finance_cutovers ON platform_finance_cutovers;
DROP TABLE IF EXISTS booking_fee_snapshots;
DROP TABLE IF EXISTS platform_finance_cutovers;
DROP FUNCTION IF EXISTS prevent_platform_finance_immutable_mutation();

COMMIT;
