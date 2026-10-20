BEGIN;
LOCK TABLE hostelhive.announcements IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.announcements WHERE audience='faculty') THEN
  RAISE EXCEPTION 'Cannot roll back faculty announcements while targeted notices exist; retain migration 29 to preserve audience privacy';
 END IF;
END $$;
CREATE OR REPLACE FUNCTION hostelhive.enqueue_announcement_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
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
ALTER TABLE hostelhive.announcements DROP CONSTRAINT announcements_faculty_check;
ALTER TABLE hostelhive.announcements DROP CONSTRAINT announcements_audience_check;
ALTER TABLE hostelhive.announcements ADD CONSTRAINT announcements_audience_check CHECK(audience='all_students');
ALTER TABLE hostelhive.announcements DROP COLUMN faculty;
COMMIT;
