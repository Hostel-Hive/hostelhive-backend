BEGIN;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.scanner_device_audit WHERE action='reactivated') THEN
  RAISE EXCEPTION 'Cannot roll back scanner reactivation while reactivation audit history exists';
 END IF;
END $$;
ALTER TABLE hostelhive.scanner_device_audit DROP CONSTRAINT scanner_device_audit_action_check;
ALTER TABLE hostelhive.scanner_device_audit ADD CONSTRAINT scanner_device_audit_action_check
 CHECK(action IN ('registered','rotated','deactivated','direction_set'));
COMMIT;
