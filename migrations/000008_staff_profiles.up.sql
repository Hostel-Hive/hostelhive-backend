BEGIN;
CREATE TABLE hostelhive.staff_profiles (
 staff_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID NOT NULL UNIQUE REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 full_name VARCHAR(200) NOT NULL CHECK(full_name=btrim(full_name) AND full_name ~ '[^[:space:]]' AND full_name !~ '[[:cntrl:]]'),
 designation VARCHAR(120) NOT NULL CHECK(designation=btrim(designation) AND designation ~ '[^[:space:]]' AND designation !~ '[[:cntrl:]]'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER staff_profiles_set_updated_at BEFORE UPDATE ON hostelhive.staff_profiles
FOR EACH ROW EXECUTE FUNCTION hostelhive.users_touch_updated_at();
-- Current-role projection: former profiles remain stored but do not supply
-- the name or designation for a different current role.
CREATE VIEW hostelhive.account_profiles AS
SELECT u.user_id,u.firebase_uid,u.email,u.role,u.is_active,
 CASE WHEN u.role='student' THEN COALESCE(s.full_name,'') ELSE COALESCE(f.full_name,'') END AS full_name,
 CASE WHEN u.role='student' THEN '' ELSE COALESCE(f.designation,'') END AS designation
FROM hostelhive.users u
LEFT JOIN hostelhive.students s ON s.user_id=u.user_id AND s.deleted_at IS NULL
LEFT JOIN hostelhive.staff_profiles f ON f.user_id=u.user_id;
COMMIT;
