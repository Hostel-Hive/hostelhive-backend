BEGIN;
ALTER TABLE hostelhive.scanner_devices ADD COLUMN direction VARCHAR(3) CHECK(direction IN ('IN','OUT'));
ALTER TABLE hostelhive.scanner_device_audit DROP CONSTRAINT scanner_device_audit_action_check;
ALTER TABLE hostelhive.scanner_device_audit ADD CONSTRAINT scanner_device_audit_action_check CHECK(action IN ('registered','rotated','deactivated','direction_set'));
ALTER TABLE hostelhive.scanner_device_audit ADD COLUMN direction VARCHAR(3) CHECK(direction IN ('IN','OUT'));
CREATE TABLE hostelhive.attendance_events (
 event_id UUID NOT NULL,
 device_id UUID NOT NULL REFERENCES hostelhive.scanner_devices(device_id) ON DELETE RESTRICT,
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 payload_hash BYTEA NOT NULL CHECK(octet_length(payload_hash)=32),
 direction VARCHAR(3) NOT NULL CHECK(direction IN ('IN','OUT')),
 outcome VARCHAR(16) NOT NULL CHECK(outcome IN ('recorded','cooldown','already_in','already_out')),
 occurred_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(device_id,event_id)
);
CREATE INDEX attendance_events_student_history ON hostelhive.attendance_events(student_id,occurred_at DESC,device_id,event_id);
CREATE TABLE hostelhive.student_presence (
 student_id UUID PRIMARY KEY REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 status VARCHAR(3) NOT NULL CHECK(status IN ('IN','OUT')),
 updated_at TIMESTAMPTZ NOT NULL,
 device_id UUID NOT NULL,
 event_id UUID NOT NULL,
 FOREIGN KEY(device_id,event_id) REFERENCES hostelhive.attendance_events(device_id,event_id) ON DELETE RESTRICT
);
COMMIT;
