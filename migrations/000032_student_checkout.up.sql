BEGIN;
ALTER TABLE hostelhive.bed_allocations DROP CONSTRAINT bed_allocations_end_reason_check;
ALTER TABLE hostelhive.bed_allocations ADD CONSTRAINT bed_allocations_end_reason_check
 CHECK(end_reason IN ('transferred','revoked','checkout'));

CREATE TABLE hostelhive.student_checkouts (
 checkout_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id UUID NOT NULL UNIQUE,
 payload_hash BYTEA NOT NULL CHECK(octet_length(payload_hash)=32),
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 user_id UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 reason VARCHAR(32) NOT NULL CHECK(reason IN ('graduated','withdrawn','moved_out','accommodation_cancelled','other')),
 notes VARCHAR(1000) NOT NULL DEFAULT '',
 deactivate_account BOOLEAN NOT NULL,
 account_active_after BOOLEAN NOT NULL,
 profile_was_archived BOOLEAN NOT NULL,
 archived_at TIMESTAMPTZ NOT NULL,
 released_allocation_id UUID UNIQUE REFERENCES hostelhive.bed_allocations(allocation_id) ON DELETE RESTRICT,
 released_bed_id UUID REFERENCES hostelhive.beds(bed_id) ON DELETE RESTRICT,
 performed_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 performed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK((released_allocation_id IS NULL)=(released_bed_id IS NULL)),
 CHECK(NOT deactivate_account OR NOT account_active_after)
);
CREATE INDEX student_checkouts_history ON hostelhive.student_checkouts(student_id,performed_at,checkout_id);
COMMIT;
