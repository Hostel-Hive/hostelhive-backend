-- Run only on the disposable database created by scripts/test-migrations.ps1.
-- Roll back fixtures so verification never leaves seed users behind.
BEGIN;

INSERT INTO hostelhive.users (firebase_uid, email, role)
VALUES
    ('uid-admin', 'admin@example.test', 'admin'),
    ('uid-warden', 'warden@example.test', 'warden'),
    ('uid-subwarden', 'subwarden@example.test', 'sub_warden'),
    ('uid-security', 'security@example.test', 'security_staff'),
    ('uid-student', 'Student@example.test', 'student'),
    ('CaseSensitiveUID', 'case-one@example.test', 'student'),
    ('casesensitiveuid', 'case-two@example.test', 'student'),
    (repeat('x', 128), 'longuid@example.test', 'student');

DO $$
DECLARE
    old_timestamp TIMESTAMPTZ;
BEGIN
    IF (SELECT count(*) FROM hostelhive.users) <> 8 THEN
        RAISE EXCEPTION 'Valid roles and case-sensitive Firebase UIDs must be accepted';
    END IF;
    IF EXISTS (SELECT 1 FROM hostelhive.users WHERE is_active OR user_id IS NULL
               OR created_at IS NULL OR updated_at IS NULL) THEN
        RAISE EXCEPTION 'Accounts must default to inactive with generated IDs and timestamps';
    END IF;
    IF (SELECT count(DISTINCT user_id) FROM hostelhive.users) <> 8 THEN
        RAISE EXCEPTION 'Generated primary keys must be unique';
    END IF;

    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('uid-student', 'duplicate-uid@example.test', 'student');
        RAISE EXCEPTION 'Duplicate Firebase UID was accepted';
    EXCEPTION WHEN unique_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('duplicate-email', 'student@EXAMPLE.TEST', 'student');
        RAISE EXCEPTION 'Case-only duplicate email was accepted';
    EXCEPTION WHEN unique_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (user_id, firebase_uid, email, role)
        SELECT user_id, 'duplicate-id', 'duplicate-id@example.test', 'student'
        FROM hostelhive.users WHERE firebase_uid = 'uid-student';
        RAISE EXCEPTION 'Duplicate primary key was accepted';
    EXCEPTION WHEN unique_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES (NULL, 'missing-uid@example.test', 'student');
        RAISE EXCEPTION 'NULL Firebase UID was accepted';
    EXCEPTION WHEN not_null_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('', 'empty-uid@example.test', 'student');
        RAISE EXCEPTION 'Empty Firebase UID was accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES (E' \t\n', 'blank-uid@example.test', 'student');
        RAISE EXCEPTION 'Whitespace-only Firebase UID was accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES (repeat('x', 129), 'too-long-uid@example.test', 'student');
        RAISE EXCEPTION 'Oversized Firebase UID was accepted';
    EXCEPTION WHEN string_data_right_truncation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('missing-role', 'missing-role@example.test', NULL);
        RAISE EXCEPTION 'NULL role was accepted';
    EXCEPTION WHEN not_null_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('invalid-role', 'invalid-role@example.test', 'superuser');
        RAISE EXCEPTION 'Unknown role was accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE hostelhive.users SET role = 'scanner' WHERE firebase_uid = 'uid-student';
        RAISE EXCEPTION 'Invalid role update was accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        UPDATE hostelhive.users SET firebase_uid = 'uid-admin' WHERE firebase_uid = 'uid-student';
        RAISE EXCEPTION 'Duplicate Firebase UID update was accepted';
    EXCEPTION WHEN unique_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role, is_active)
        VALUES ('missing-status', 'missing-status@example.test', 'student', NULL);
        RAISE EXCEPTION 'NULL active status was accepted';
    EXCEPTION WHEN not_null_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('missing-email', NULL, 'student');
        RAISE EXCEPTION 'NULL email was accepted';
    EXCEPTION WHEN not_null_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('bad-email', 'not-an-email', 'student');
        RAISE EXCEPTION 'Malformed email was accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;
    BEGIN
        INSERT INTO hostelhive.users (firebase_uid, email, role)
        VALUES ('spaced-email', ' student@example.test ', 'student');
        RAISE EXCEPTION 'Untrimmed email was accepted';
    EXCEPTION WHEN check_violation THEN NULL;
    END;

    SELECT updated_at INTO old_timestamp FROM hostelhive.users WHERE firebase_uid = 'uid-student';
    UPDATE hostelhive.users SET is_active = TRUE WHERE firebase_uid = 'uid-student';
    IF NOT (SELECT is_active AND updated_at > old_timestamp
            FROM hostelhive.users WHERE firebase_uid = 'uid-student') THEN
        RAISE EXCEPTION 'Activation and automatic update timestamp failed';
    END IF;
    UPDATE hostelhive.users SET is_active = FALSE WHERE firebase_uid = 'uid-student';
    IF (SELECT is_active FROM hostelhive.users WHERE firebase_uid = 'uid-student') THEN
        RAISE EXCEPTION 'Deactivation failed';
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'hostelhive' AND table_name = 'users'
        AND (column_name LIKE '%password%' OR column_name LIKE '%token%')
    ) THEN
        RAISE EXCEPTION 'Local credentials or tokens must not be stored';
    END IF;
END;
$$;

ROLLBACK;
