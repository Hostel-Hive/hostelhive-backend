BEGIN;
ALTER TABLE hostelhive.announcements ADD COLUMN faculty VARCHAR(120);
ALTER TABLE hostelhive.announcements DROP CONSTRAINT announcements_audience_check;
ALTER TABLE hostelhive.announcements ADD CONSTRAINT announcements_audience_check
 CHECK(audience IN ('all_students','faculty'));
ALTER TABLE hostelhive.announcements ADD CONSTRAINT announcements_faculty_check CHECK(
 (audience='all_students' AND faculty IS NULL)
 OR (audience='faculty' AND faculty IS NOT NULL AND length(btrim(faculty))>0 AND faculty=lower(btrim(faculty))));

CREATE OR REPLACE FUNCTION hostelhive.enqueue_announcement_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid UUID;
BEGIN
 INSERT INTO hostelhive.notification_events(source_type,announcement_id,kind,status,occurred_at)
 VALUES('announcement',NEW.announcement_id,'announcement_published','published',NEW.published_at) RETURNING event_id INTO eid;
 INSERT INTO hostelhive.notification_outbox(event_id,recipient_id)
 SELECT eid,u.user_id FROM hostelhive.users u JOIN hostelhive.students s ON s.user_id=u.user_id
 WHERE u.is_active AND u.role='student' AND s.deleted_at IS NULL
 AND (NEW.audience='all_students' OR (NEW.audience='faculty' AND lower(btrim(s.faculty))=NEW.faculty))
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END;
$$;
COMMIT;
