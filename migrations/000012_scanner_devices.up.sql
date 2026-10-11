BEGIN;
CREATE TABLE hostelhive.scanner_devices (
 device_id UUID PRIMARY KEY,
 name VARCHAR(120) NOT NULL CHECK(name=btrim(name) AND name ~ '[^[:space:]]'),
 credential_hash BYTEA NOT NULL CHECK(octet_length(credential_hash)=32),
 credential_version INTEGER NOT NULL DEFAULT 1 CHECK(credential_version>=1),
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 created_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 updated_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX scanner_devices_name_unique ON hostelhive.scanner_devices(lower(name));
CREATE TABLE hostelhive.scanner_device_audit (
 audit_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 device_id UUID NOT NULL REFERENCES hostelhive.scanner_devices(device_id) ON DELETE RESTRICT,
 action VARCHAR(16) NOT NULL CHECK(action IN ('registered','rotated','deactivated')),
 credential_version INTEGER NOT NULL CHECK(credential_version>=1),
 actor_id UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX scanner_device_audit_history ON hostelhive.scanner_device_audit(device_id,occurred_at,audit_id);
COMMIT;
