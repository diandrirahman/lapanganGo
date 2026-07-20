BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM booking_fee_snapshots LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot rollback: booking_fee_snapshots contains data. Refusing to drop tables to prevent data loss.';
    END IF;
    IF EXISTS (SELECT 1 FROM platform_finance_cutovers LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot rollback: platform_finance_cutovers contains data. Refusing to drop tables to prevent data loss.';
    END IF;
END $$;

DROP TRIGGER IF EXISTS prevent_mutation_booking_fee_snapshots ON booking_fee_snapshots;
DROP TRIGGER IF EXISTS prevent_mutation_platform_finance_cutovers ON platform_finance_cutovers;
DROP TABLE IF EXISTS booking_fee_snapshots;
DROP TABLE IF EXISTS platform_finance_cutovers;
DROP FUNCTION IF EXISTS prevent_platform_finance_immutable_mutation();

COMMIT;
