BEGIN;
-- Nullable for pre-existing inventory fixtures/history; new API writes set actors.
ALTER TABLE hostelhive.bed_allocations
 ADD COLUMN started_by UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 ADD COLUMN ended_by UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 ADD COLUMN end_reason VARCHAR(16) CHECK(end_reason IN ('transferred','revoked'));
COMMIT;
