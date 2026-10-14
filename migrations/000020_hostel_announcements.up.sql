BEGIN;
CREATE TABLE hostelhive.announcements (
 announcement_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id UUID NOT NULL,
 published_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 title VARCHAR(160) NOT NULL CHECK(length(btrim(title))>0),
 content VARCHAR(5000) NOT NULL CHECK(length(btrim(content))>0),
 audience VARCHAR(16) NOT NULL DEFAULT 'all_students' CHECK(audience='all_students'),
 priority VARCHAR(16) NOT NULL DEFAULT 'normal' CHECK(priority='normal'),
 published_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(published_by,request_id)
);
CREATE INDEX announcements_feed ON hostelhive.announcements(published_at DESC,announcement_id DESC);
ALTER TABLE hostelhive.notification_events ADD COLUMN announcement_id UUID UNIQUE REFERENCES hostelhive.announcements(announcement_id) ON DELETE CASCADE;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_source_type_check;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_check;
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_source_type_check CHECK(source_type IN ('complaint','leave','announcement'));
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_check CHECK(
 (source_type='complaint' AND complaint_update_id IS NOT NULL AND leave_id IS NULL AND announcement_id IS NULL
  AND kind IN ('created','status') AND status IN ('pending','in_progress','resolved'))
 OR (source_type='leave' AND complaint_update_id IS NULL AND leave_id IS NOT NULL AND announcement_id IS NULL
  AND ((kind='leave_submitted' AND status='pending') OR (kind='leave_decided' AND status IN ('approved','rejected'))))
 OR (source_type='announcement' AND complaint_update_id IS NULL AND leave_id IS NULL AND announcement_id IS NOT NULL
  AND kind='announcement_published' AND status='published'));
CREATE FUNCTION hostelhive.enqueue_announcement_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid UUID;
BEGIN
 INSERT INTO hostelhive.notification_events(source_type,announcement_id,kind,status,occurred_at)
 VALUES('announcement',NEW.announcement_id,'announcement_published','published',NEW.published_at) RETURNING event_id INTO eid;
 INSERT INTO hostelhive.notification_outbox(event_id,recipient_id)
 SELECT eid,u.user_id FROM hostelhive.users u JOIN hostelhive.students s ON s.user_id=u.user_id
 WHERE u.is_active AND u.role='student' AND s.deleted_at IS NULL
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END;
$$;
CREATE TRIGGER announcement_notifications AFTER INSERT ON hostelhive.announcements
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_announcement_notifications();
COMMIT;
