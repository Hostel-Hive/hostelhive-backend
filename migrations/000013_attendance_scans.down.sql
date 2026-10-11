BEGIN;
DROP TABLE hostelhive.student_presence;
DROP TABLE hostelhive.attendance_events;
DELETE FROM hostelhive.scanner_device_audit WHERE action='direction_set';
ALTER TABLE hostelhive.scanner_device_audit DROP COLUMN direction;
ALTER TABLE hostelhive.scanner_device_audit DROP CONSTRAINT scanner_device_audit_action_check;
ALTER TABLE hostelhive.scanner_device_audit ADD CONSTRAINT scanner_device_audit_action_check CHECK(action IN ('registered','rotated','deactivated'));
ALTER TABLE hostelhive.scanner_devices DROP COLUMN direction;
COMMIT;
