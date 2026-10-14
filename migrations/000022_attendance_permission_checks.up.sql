BEGIN;
ALTER TABLE hostelhive.attendance_events ADD COLUMN access_result VARCHAR(16);
ALTER TABLE hostelhive.attendance_events ADD COLUMN late_access_id UUID UNIQUE REFERENCES hostelhive.late_access_requests(late_access_id) ON DELETE RESTRICT;
ALTER TABLE hostelhive.attendance_events ADD CONSTRAINT attendance_access_result_check CHECK(
 (access_result IS NULL AND late_access_id IS NULL)
 OR (access_result IS NOT NULL AND outcome='recorded' AND ((access_result='approved' AND late_access_id IS NOT NULL)
   OR (access_result IN ('not_required','unapproved') AND late_access_id IS NULL)))
 OR (outcome<>'recorded' AND access_result='not_evaluated' AND late_access_id IS NULL));
ALTER TABLE hostelhive.late_access_requests ADD COLUMN consumed_at TIMESTAMPTZ;
ALTER TABLE hostelhive.late_access_requests ADD COLUMN consumed_device_id UUID;
ALTER TABLE hostelhive.late_access_requests ADD COLUMN consumed_event_id UUID;
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_consumption_check CHECK(
 (consumed_at IS NULL AND consumed_device_id IS NULL AND consumed_event_id IS NULL)
 OR (status='approved' AND consumed_at IS NOT NULL AND isfinite(consumed_at) AND consumed_device_id IS NOT NULL AND consumed_event_id IS NOT NULL));
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_consumed_event_fkey
 FOREIGN KEY(consumed_device_id,consumed_event_id) REFERENCES hostelhive.attendance_events(device_id,event_id) ON DELETE RESTRICT;
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_consumed_movement UNIQUE(consumed_device_id,consumed_event_id);
CREATE INDEX late_access_unused_approval ON hostelhive.late_access_requests(student_id,direction,expected_at)
 WHERE status='approved' AND consumed_at IS NULL;
CREATE TABLE hostelhive.attendance_incidents (
 incident_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 device_id UUID NOT NULL,
 event_id UUID NOT NULL,
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 direction VARCHAR(3) NOT NULL CHECK(direction IN ('IN','OUT')),
 incident_type VARCHAR(10) NOT NULL CHECK((direction='IN' AND incident_type='late_entry') OR (direction='OUT' AND incident_type='early_exit')),
 occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
 hostel_timezone TEXT NOT NULL CHECK(length(hostel_timezone)>0),
 exit_opening_minute INTEGER NOT NULL CHECK(exit_opening_minute BETWEEN 0 AND 1439),
 entry_deadline_minute INTEGER NOT NULL CHECK(entry_deadline_minute BETWEEN 0 AND 1439),
 permission_window_seconds INTEGER NOT NULL CHECK(permission_window_seconds=1800),
 UNIQUE(device_id,event_id),
 FOREIGN KEY(device_id,event_id) REFERENCES hostelhive.attendance_events(device_id,event_id) ON DELETE CASCADE
);
CREATE INDEX attendance_incidents_history ON hostelhive.attendance_incidents(occurred_at DESC,incident_id DESC);
CREATE INDEX attendance_incidents_student ON hostelhive.attendance_incidents(student_id,occurred_at DESC,incident_id DESC);
ALTER TABLE hostelhive.notification_events ADD COLUMN attendance_incident_id UUID UNIQUE REFERENCES hostelhive.attendance_incidents(incident_id) ON DELETE CASCADE;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_check;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_source_type_check;
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_source_type_check CHECK(source_type IN ('complaint','leave','announcement','late_access','attendance'));
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_check CHECK(
 (source_type='complaint' AND complaint_update_id IS NOT NULL AND leave_id IS NULL AND announcement_id IS NULL AND late_access_id IS NULL AND attendance_incident_id IS NULL
  AND kind IN ('created','status') AND status IN ('pending','in_progress','resolved'))
 OR (source_type='leave' AND complaint_update_id IS NULL AND leave_id IS NOT NULL AND announcement_id IS NULL AND late_access_id IS NULL AND attendance_incident_id IS NULL
  AND ((kind='leave_submitted' AND status='pending') OR (kind='leave_decided' AND status IN ('approved','rejected'))))
 OR (source_type='announcement' AND complaint_update_id IS NULL AND leave_id IS NULL AND announcement_id IS NOT NULL AND late_access_id IS NULL AND attendance_incident_id IS NULL
  AND kind='announcement_published' AND status='published')
 OR (source_type='late_access' AND complaint_update_id IS NULL AND leave_id IS NULL AND announcement_id IS NULL AND late_access_id IS NOT NULL AND attendance_incident_id IS NULL
  AND ((kind='late_access_submitted' AND status='pending') OR (kind='late_access_decided' AND status IN ('approved','rejected'))))
 OR (source_type='attendance' AND complaint_update_id IS NULL AND leave_id IS NULL AND announcement_id IS NULL AND late_access_id IS NULL AND attendance_incident_id IS NOT NULL
  AND kind='attendance_incident' AND status='unapproved'));
CREATE FUNCTION hostelhive.enqueue_attendance_incident_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid UUID;
BEGIN
 INSERT INTO hostelhive.notification_events(source_type,attendance_incident_id,kind,status,occurred_at)
 VALUES('attendance',NEW.incident_id,'attendance_incident','unapproved',NEW.occurred_at) RETURNING event_id INTO eid;
 INSERT INTO hostelhive.notification_outbox(event_id,recipient_id)
 SELECT eid,u.user_id FROM hostelhive.users u WHERE u.is_active AND u.role IN ('admin','warden')
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END;
$$;
CREATE TRIGGER attendance_incident_notifications AFTER INSERT ON hostelhive.attendance_incidents
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_attendance_incident_notifications();
-- Historical scans and approvals are not reclassified or consumed.
COMMIT;