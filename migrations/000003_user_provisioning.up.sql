BEGIN;

-- Durable retry metadata only: no passwords, password hashes or session tokens.
CREATE TABLE hostelhive.user_provisioning (
    request_key VARCHAR(128) COLLATE "C" PRIMARY KEY,
    requester_uid VARCHAR(128) COLLATE "C" NOT NULL,
    firebase_uid VARCHAR(128) COLLATE "C" NOT NULL UNIQUE,
    email VARCHAR(254) NOT NULL,
    role VARCHAR(32) NOT NULL,
    user_id UUID UNIQUE REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT provisioning_key_valid CHECK (request_key ~ '^[A-Za-z0-9_-]{8,128}$'),
    CONSTRAINT provisioning_requester_nonblank CHECK (requester_uid ~ '[^[:space:]]'),
    CONSTRAINT provisioning_uid_nonblank CHECK (firebase_uid ~ '[^[:space:]]'),
    CONSTRAINT provisioning_email_valid CHECK (email = lower(btrim(email)) AND email ~ '^[^[:space:]@]+@[^[:space:]@]+$'),
    CONSTRAINT provisioning_role_valid CHECK (role IN ('admin', 'warden', 'sub_warden', 'security_staff', 'student')),
    CONSTRAINT provisioning_completion_consistent CHECK ((user_id IS NULL) = (completed_at IS NULL))
);
CREATE UNIQUE INDEX provisioning_email_unique ON hostelhive.user_provisioning(email);

COMMIT;
