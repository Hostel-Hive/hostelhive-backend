BEGIN;
-- Opaque, stable identifiers only; never Firebase credentials or personal data.
CREATE TABLE hostelhive.student_qr (
 student_id UUID PRIMARY KEY REFERENCES hostelhive.students(student_id) ON DELETE CASCADE,
 token VARCHAR(64) NOT NULL UNIQUE CHECK(token ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
COMMIT;
