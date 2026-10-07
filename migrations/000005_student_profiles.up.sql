BEGIN;
CREATE TABLE hostelhive.students (
 student_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID NOT NULL UNIQUE REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 index_no VARCHAR(64) NOT NULL CHECK(index_no=btrim(index_no) AND index_no ~ '[^[:space:]]'),
 full_name VARCHAR(200) NOT NULL CHECK(full_name=btrim(full_name) AND full_name ~ '[^[:space:]]'),
 faculty VARCHAR(120) NOT NULL CHECK(faculty=btrim(faculty) AND faculty ~ '[^[:space:]]'),
 year SMALLINT NOT NULL CHECK(year BETWEEN 1 AND 10),
 contact_phone VARCHAR(32) NOT NULL CHECK(contact_phone ~ '^[+()0-9 -]{7,32}$' AND length(regexp_replace(contact_phone,'[^0-9]','','g'))>=7),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 deleted_at TIMESTAMPTZ,
 deleted_by UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 CHECK((deleted_at IS NULL)=(deleted_by IS NULL))
);
-- Archived identifiers remain reserved to protect historical identity.
CREATE UNIQUE INDEX students_index_no_unique ON hostelhive.students(lower(index_no));
CREATE INDEX students_current_order ON hostelhive.students(created_at,student_id) WHERE deleted_at IS NULL;
CREATE TABLE hostelhive.guardians (
 guardian_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 name VARCHAR(200) NOT NULL CHECK(name=btrim(name) AND name ~ '[^[:space:]]'),
 relationship VARCHAR(80) NOT NULL CHECK(relationship=btrim(relationship) AND relationship ~ '[^[:space:]]'),
 contact_phone VARCHAR(32) NOT NULL CHECK(contact_phone ~ '^[+()0-9 -]{7,32}$' AND length(regexp_replace(contact_phone,'[^0-9]','','g'))>=7)
);
CREATE INDEX guardians_student ON hostelhive.guardians(student_id,guardian_id);
CREATE TRIGGER students_set_updated_at BEFORE UPDATE ON hostelhive.students
FOR EACH ROW EXECUTE FUNCTION hostelhive.users_touch_updated_at();
COMMIT;
