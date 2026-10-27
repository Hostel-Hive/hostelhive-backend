BEGIN;
-- Never discard or mislabel a recorded physical movement to permit rollback.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM hostelhive.attendance_incidents WHERE incident_type='late_exit') THEN
  RAISE EXCEPTION 'Cannot roll back hostel movement policy while late-exit incidents exist';
 END IF;
END $$;
ALTER TABLE hostelhive.attendance_incidents DROP CONSTRAINT attendance_incidents_check;
ALTER TABLE hostelhive.attendance_incidents ADD CONSTRAINT attendance_incidents_check
 CHECK ((direction='IN' AND incident_type='late_entry')
     OR (direction='OUT' AND incident_type='early_exit'));
COMMIT;
