BEGIN;
-- Existing early_exit/late_entry classifications and notification IDs stay intact.
ALTER TABLE hostelhive.attendance_incidents DROP CONSTRAINT attendance_incidents_check;
ALTER TABLE hostelhive.attendance_incidents ADD CONSTRAINT attendance_incidents_check
 CHECK ((direction='IN' AND incident_type='late_entry')
     OR (direction='OUT' AND incident_type IN ('early_exit','late_exit')));
COMMIT;
