BEGIN;
-- A staff member may create their name before an Admin assigns a designation.
-- Empty means unassigned; designation never controls authorization.
ALTER TABLE hostelhive.staff_profiles DROP CONSTRAINT staff_profiles_designation_check;
ALTER TABLE hostelhive.staff_profiles ALTER COLUMN designation SET DEFAULT '';
ALTER TABLE hostelhive.staff_profiles ADD CONSTRAINT staff_profiles_designation_check
 CHECK(designation=btrim(designation) AND designation !~ '[[:cntrl:]]');
COMMIT;
