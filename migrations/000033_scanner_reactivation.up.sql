BEGIN;
ALTER TABLE hostelhive.scanner_device_audit DROP CONSTRAINT scanner_device_audit_action_check;
ALTER TABLE hostelhive.scanner_device_audit ADD CONSTRAINT scanner_device_audit_action_check
 CHECK(action IN ('registered','rotated','deactivated','direction_set','reactivated'));
COMMIT;
