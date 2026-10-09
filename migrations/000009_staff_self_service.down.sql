BEGIN;
-- Do not erase profiles or invent an administrative designation on rollback.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.staff_profiles WHERE designation='') THEN
  RAISE EXCEPTION 'Assign real designations to incomplete staff profiles before reverting staff self-service';
 END IF;
END $$;
ALTER TABLE hostelhive.staff_profiles DROP CONSTRAINT staff_profiles_designation_check;
ALTER TABLE hostelhive.staff_profiles ALTER COLUMN designation DROP DEFAULT;
ALTER TABLE hostelhive.staff_profiles ADD CONSTRAINT staff_profiles_designation_check
 CHECK(designation=btrim(designation) AND designation ~ '[^[:space:]]' AND designation !~ '[[:cntrl:]]');
COMMIT;
