BEGIN;
CREATE TABLE hostelhive.leave_requests (
 leave_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id UUID NOT NULL,
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 submitted_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 starts_at TIMESTAMPTZ NOT NULL CHECK(isfinite(starts_at)),
 ends_at TIMESTAMPTZ NOT NULL CHECK(isfinite(ends_at) AND ends_at > starts_at),
 reason VARCHAR(1000) NOT NULL CHECK(reason=btrim(reason) AND length(reason)>0),
 status VARCHAR(8) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 decided_at TIMESTAMPTZ,
 decided_by UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 decision_note VARCHAR(1000) NOT NULL DEFAULT '',
 UNIQUE(student_id,request_id),
 CHECK((status='pending' AND decided_at IS NULL AND decided_by IS NULL AND decision_note='') OR
       (status IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL)),
 CHECK(status<>'rejected' OR length(btrim(decision_note))>0)
);
CREATE INDEX leave_requests_student_history ON hostelhive.leave_requests(student_id,created_at DESC,leave_id DESC);
CREATE INDEX leave_requests_pending ON hostelhive.leave_requests(created_at,leave_id) WHERE status='pending';
COMMIT;
