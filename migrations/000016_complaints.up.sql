BEGIN;
CREATE TABLE hostelhive.complaints (
 complaint_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id UUID NOT NULL,
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 submitted_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 category VARCHAR(16) NOT NULL CHECK(category IN ('maintenance','disciplinary')),
 description VARCHAR(2000) NOT NULL CHECK(length(btrim(description))>0),
 status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','in_progress','resolved')),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(student_id,request_id)
);
CREATE INDEX complaints_history ON hostelhive.complaints(created_at DESC,complaint_id DESC);
CREATE INDEX complaints_student ON hostelhive.complaints(student_id,created_at DESC,complaint_id DESC);
CREATE TABLE hostelhive.complaint_updates (
 update_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 complaint_id UUID NOT NULL REFERENCES hostelhive.complaints(complaint_id) ON DELETE RESTRICT,
 request_id UUID NOT NULL,
 actor_id UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 action VARCHAR(16) NOT NULL CHECK(action IN ('created','note','status')),
 previous_status VARCHAR(16) CHECK(previous_status IN ('pending','in_progress','resolved')),
 new_status VARCHAR(16) NOT NULL CHECK(new_status IN ('pending','in_progress','resolved')),
 text VARCHAR(2000) NOT NULL DEFAULT '',
 previous_version BIGINT NOT NULL CHECK(previous_version>=0),
 version BIGINT NOT NULL CHECK(version=previous_version+1),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(complaint_id,actor_id,request_id),
 UNIQUE(complaint_id,version)
);
CREATE INDEX complaint_updates_history ON hostelhive.complaint_updates(complaint_id,version);
COMMIT;
