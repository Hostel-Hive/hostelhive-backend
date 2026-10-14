BEGIN;
DROP TRIGGER announcement_notifications ON hostelhive.announcements;
DROP FUNCTION hostelhive.enqueue_announcement_notifications();
-- Only announcement intent/inbox entries are removed by downgrade.
DELETE FROM hostelhive.notification_events WHERE source_type='announcement';
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_check;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_source_type_check;
ALTER TABLE hostelhive.notification_events DROP COLUMN announcement_id;
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_source_type_check CHECK(source_type IN ('complaint','leave'));
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_check CHECK(
 (source_type='complaint' AND complaint_update_id IS NOT NULL AND leave_id IS NULL
  AND kind IN ('created','status') AND status IN ('pending','in_progress','resolved'))
 OR (source_type='leave' AND complaint_update_id IS NULL AND leave_id IS NOT NULL
  AND ((kind='leave_submitted' AND status='pending') OR (kind='leave_decided' AND status IN ('approved','rejected')))));
DROP TABLE hostelhive.announcements;
COMMIT;
