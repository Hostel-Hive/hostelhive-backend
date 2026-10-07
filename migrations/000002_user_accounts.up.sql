-- USER entity from SDS chapter 2, amended for Firebase identity under ADR-002.
BEGIN;

CREATE TABLE hostelhive.users (
    user_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    firebase_uid VARCHAR(128) COLLATE "C" NOT NULL,
    email VARCHAR(254) NOT NULL,
    role VARCHAR(32) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT users_firebase_uid_unique UNIQUE (firebase_uid),
    CONSTRAINT users_firebase_uid_nonblank CHECK (firebase_uid ~ '[^[:space:]]'),
    CONSTRAINT users_email_valid CHECK (
        email = btrim(email) AND email ~ '^[^[:space:]@]+@[^[:space:]@]+$'
    ),
    CONSTRAINT users_role_valid CHECK (
        role IN ('admin', 'warden', 'sub_warden', 'security_staff', 'student')
    )
);

-- Preserve the SDS email candidate key without allowing case-only duplicates.
CREATE UNIQUE INDEX users_email_unique ON hostelhive.users (lower(email));

CREATE FUNCTION hostelhive.users_touch_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at := clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_set_updated_at
BEFORE UPDATE ON hostelhive.users
FOR EACH ROW EXECUTE FUNCTION hostelhive.users_touch_updated_at();

COMMIT;
