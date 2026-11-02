BEGIN;
LOCK TABLE hostelhive.faculties, hostelhive.faculty_names, hostelhive.faculty_audit IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.faculty_audit)
 OR EXISTS(SELECT 1 FROM hostelhive.faculties WHERE NOT is_active)
 OR EXISTS(SELECT 1 FROM hostelhive.faculty_names n JOIN hostelhive.faculties f USING(faculty_id) WHERE n.name_key<>lower(f.name)) THEN
  RAISE EXCEPTION 'Cannot roll back faculty management after lifecycle changes; retain migration 34 to preserve faculty identity and announcement privacy';
 END IF;
END $$;
DROP TABLE hostelhive.faculty_audit;
DROP TABLE hostelhive.faculty_names;
DROP TABLE hostelhive.faculties;
COMMIT;
