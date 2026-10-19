BEGIN;
ALTER TABLE hostelhive.students ADD CONSTRAINT student_account_pair UNIQUE(student_id,user_id);
CREATE TABLE hostelhive.student_invitations (
 invitation_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id UUID NOT NULL CHECK(request_id<>'00000000-0000-0000-0000-000000000000'),
 user_id UUID NOT NULL UNIQUE REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 student_id UUID NOT NULL UNIQUE,
 email VARCHAR(254) NOT NULL UNIQUE CHECK(email=lower(btrim(email))),
 created_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 request_ip INET NOT NULL,
 payload_hash BYTEA NOT NULL CHECK(octet_length(payload_hash)=32),
 baseline_password_updated_at DOUBLE PRECISION CHECK(baseline_password_updated_at>0 AND baseline_password_updated_at<1e16),
 delivery_status VARCHAR(8) NOT NULL DEFAULT 'pending' CHECK(delivery_status IN ('pending','sent','failed','skipped')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 8),
 last_error VARCHAR(32) NOT NULL DEFAULT '' CHECK(last_error IN ('','identity_conflict','provider_unavailable','ineligible')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 last_requested_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 next_attempt_at TIMESTAMPTZ DEFAULT clock_timestamp(),
 last_sent_at TIMESTAMPTZ,
 setup_completed_at TIMESTAMPTZ,
 UNIQUE(created_by,request_id),
 FOREIGN KEY(student_id,user_id) REFERENCES hostelhive.students(student_id,user_id) ON DELETE RESTRICT
);
CREATE INDEX student_invitations_due ON hostelhive.student_invitations(next_attempt_at,invitation_id) WHERE delivery_status='pending';
CREATE TABLE hostelhive.student_invitation_resends (
 invitation_id UUID NOT NULL REFERENCES hostelhive.student_invitations(invitation_id) ON DELETE CASCADE,
 request_id UUID NOT NULL CHECK(request_id<>'00000000-0000-0000-0000-000000000000'),
 actor_id UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(invitation_id,request_id)
);
CREATE TABLE hostelhive.student_invitation_audit (
 audit_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 invitation_id UUID NOT NULL REFERENCES hostelhive.student_invitations(invitation_id) ON DELETE CASCADE,
 actor_id UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 action VARCHAR(16) NOT NULL CHECK(action IN ('requested','prepared','sent','resend_requested','completed','failed','skipped')),
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
COMMIT;
