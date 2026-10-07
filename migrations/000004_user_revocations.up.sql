BEGIN;
CREATE TABLE hostelhive.user_revocations (
 user_id UUID PRIMARY KEY REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 requested_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 attempts BIGINT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 completed_at TIMESTAMPTZ
);
CREATE INDEX user_revocations_pending ON hostelhive.user_revocations(next_attempt_at) WHERE completed_at IS NULL;
COMMIT;
