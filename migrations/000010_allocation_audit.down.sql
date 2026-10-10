BEGIN;
ALTER TABLE hostelhive.bed_allocations DROP COLUMN end_reason, DROP COLUMN ended_by, DROP COLUMN started_by;
COMMIT;
