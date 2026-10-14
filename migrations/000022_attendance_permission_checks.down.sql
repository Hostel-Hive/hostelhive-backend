BEGIN;
DROP TRIGGER attendance_incident_notifications ON hostelhive.attendance_incidents;
DROP FUNCTION hostelhive.enqueue_attendance_incident_notifications();
DELETE FROM hostelhive.notification_events WHERE source_type='attendance';
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_check;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_source_type_check;
ALTER TABLE hostelhive.notification_events DROP COLUMN attendance_incident_id;
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_source_type_check CHECK(source_type IN ('complaint','leave','announcement','late_access'));
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_check CHECK(
 (source_type='complaint' AND complaint_update_id IS NOT NULL AND leave_id IS NULL AND announcement_id IS NULL AND late_access_id IS NULL
  AND kind IN ('created','status') AND status IN ('pending','in_progress','resolved'))
 OR (source_type='leave' AND complaint_update_id IS NULL AND leave_id IS NOT NULL AND announcement_id IS NULL AND late_access_id IS NULL
  AND ((kind='leave_submitted' AND status='pending') OR (kind='leave_decided' AND status IN ('approved','rejected'))))
 OR (source_type='announcement' AND complaint_update_id IS NULL AND leave_id IS NULL AND announcement_id IS NOT NULL AND late_access_id IS NULL
  AND kind='announcement_published' AND status='published')
 OR (source_type='late_access' AND complaint_update_id IS NULL AND leave_id IS NULL AND announcement_id IS NULL AND late_access_id IS NOT NULL
  AND ((kind='late_access_submitted' AND status='pending') OR (kind='late_access_decided' AND status IN ('approved','rejected')))));
DROP TABLE hostelhive.attendance_incidents;
DROP INDEX hostelhive.late_access_unused_approval;
ALTER TABLE hostelhive.late_access_requests DROP CONSTRAINT late_access_consumption_check;
ALTER TABLE hostelhive.late_access_requests DROP COLUMN consumed_at,DROP COLUMN consumed_device_id,DROP COLUMN consumed_event_id;
ALTER TABLE hostelhive.attendance_events DROP CONSTRAINT attendance_access_result_check;
ALTER TABLE hostelhive.attendance_events DROP COLUMN access_result,DROP COLUMN late_access_id;
COMMIT;