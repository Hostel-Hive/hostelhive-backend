BEGIN;
LOCK TABLE hostelhive.students, hostelhive.announcements IN SHARE ROW EXCLUSIVE MODE;
CREATE TABLE hostelhive.faculties (
 faculty_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 name VARCHAR(120) NOT NULL CHECK(name=btrim(name) AND length(name)>0 AND name !~ '[[:cntrl:]]'),
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 created_by UUID REFERENCES hostelhive.users(user_id) ON DELETE SET NULL,
 updated_by UUID REFERENCES hostelhive.users(user_id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX faculties_name_unique ON hostelhive.faculties(lower(name));
-- Historical names stay reserved to the same identity so renaming cannot leak
-- an old announcement to a newly created, unrelated faculty with the old name.
CREATE TABLE hostelhive.faculty_names (
 name_key VARCHAR(120) PRIMARY KEY CHECK(name_key=lower(btrim(name_key)) AND length(name_key)>0),
 faculty_id UUID NOT NULL REFERENCES hostelhive.faculties(faculty_id) ON DELETE CASCADE
);
CREATE INDEX faculty_names_identity ON hostelhive.faculty_names(faculty_id);
CREATE TABLE hostelhive.faculty_audit (
 audit_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 faculty_id UUID NOT NULL REFERENCES hostelhive.faculties(faculty_id) ON DELETE CASCADE,
 actor_id UUID REFERENCES hostelhive.users(user_id) ON DELETE SET NULL,
 action TEXT NOT NULL CHECK(action IN ('created','renamed','deactivated','activated')),
 old_name VARCHAR(120),
 new_name VARCHAR(120) NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
-- Import all existing spellings, including archived students and historical
-- announcements. No made-up default faculties are introduced.
INSERT INTO hostelhive.faculties(name)
SELECT min(btrim(name)) FROM (
 SELECT faculty AS name FROM hostelhive.students
 UNION ALL SELECT faculty FROM hostelhive.announcements WHERE faculty IS NOT NULL
) names WHERE length(btrim(name))>0 GROUP BY lower(btrim(name));
INSERT INTO hostelhive.faculty_names(name_key,faculty_id)
SELECT lower(name),faculty_id FROM hostelhive.faculties;
COMMIT;
