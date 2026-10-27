BEGIN;
CREATE TABLE hostelhive.student_profile_restorations (
 restoration_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 archived_at TIMESTAMPTZ NOT NULL,
 archived_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 restored_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 restored_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT
);
CREATE INDEX student_restorations_history ON hostelhive.student_profile_restorations(student_id,restored_at,restoration_id);
COMMIT;
