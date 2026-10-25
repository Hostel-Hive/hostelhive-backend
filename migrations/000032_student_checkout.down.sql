BEGIN;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.student_checkouts)
    OR EXISTS(SELECT 1 FROM hostelhive.bed_allocations WHERE end_reason='checkout') THEN
  RAISE EXCEPTION 'Cannot roll back student checkout while departure audit or checkout allocation history exists';
 END IF;
END $$;
DROP TABLE hostelhive.student_checkouts;
ALTER TABLE hostelhive.bed_allocations DROP CONSTRAINT bed_allocations_end_reason_check;
ALTER TABLE hostelhive.bed_allocations ADD CONSTRAINT bed_allocations_end_reason_check
 CHECK(end_reason IN ('transferred','revoked'));
COMMIT;
