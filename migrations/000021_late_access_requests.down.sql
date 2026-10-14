BEGIN;
DROP TRIGGER late_access_notifications ON hostelhive.late_access_requests;
DROP FUNCTION hostelhive.enqueue_late_access_notifications();
-- Downgrade removes only late-access requests and their delivery/read records.
DELETE FROM hostelhive.notification_events WHERE source_type='late_access';
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_check;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_source_type_check;
ALTER TABLE hostelhive.notification_events DROP COLUMN late_access_id;
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_source_type_check CHECK(source_type IN ('complaint','leave','announcement'));
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_check CHECK(
 (source_type='complaint' AND complaint_update_id IS NOT NULL AND leave_id IS NULL AND announcement_id IS NULL
  AND kind IN ('created','status') AND status IN ('pending','in_progress','resolved'))
 OR (source_type='leave' AND complaint_update_id IS NULL AND leave_id IS NOT NULL AND announcement_id IS NULL
  AND ((kind='leave_submitted' AND status='pending') OR (kind='leave_decided' AND status IN ('approved','rejected'))))
 OR (source_type='announcement' AND complaint_update_id IS NULL AND leave_id IS NULL AND announcement_id IS NOT NULL
  AND kind='announcement_published' AND status='published'));

DROP TABLE hostelhive.late_access_requests;
COMMIT;