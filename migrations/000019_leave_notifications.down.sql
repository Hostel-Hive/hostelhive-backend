BEGIN;
DROP TRIGGER leave_notifications ON hostelhive.leave_requests;
DROP FUNCTION hostelhive.enqueue_leave_notifications();
-- Downgrading removes leave alerts only; preserve the complete complaint inbox.
DELETE FROM hostelhive.notification_events WHERE source_type='leave';
ALTER TABLE hostelhive.notification_outbox DROP CONSTRAINT notification_outbox_event_id_fkey;
ALTER TABLE hostelhive.notification_outbox ADD CONSTRAINT notification_outbox_event_id_fkey
 FOREIGN KEY(event_id) REFERENCES hostelhive.complaint_updates(update_id) ON DELETE CASCADE;
CREATE OR REPLACE FUNCTION hostelhive.enqueue_complaint_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c hostelhive.complaints%ROWTYPE;
BEGIN
 IF NEW.action NOT IN ('created','status') THEN RETURN NEW; END IF;
 SELECT * INTO STRICT c FROM hostelhive.complaints WHERE complaint_id=NEW.complaint_id;
 IF NEW.action='created' THEN
  INSERT INTO hostelhive.notification_outbox(event_id,recipient_id)
  SELECT NEW.update_id,u.user_id FROM hostelhive.users u
  WHERE u.is_active AND (u.role='warden' OR (u.role='sub_warden' AND c.category='maintenance'))
  ON CONFLICT DO NOTHING;
 ELSE
  INSERT INTO hostelhive.notification_outbox(event_id,recipient_id)
  SELECT NEW.update_id,s.user_id FROM hostelhive.students s WHERE s.student_id=c.student_id
  ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$;
DROP TABLE hostelhive.notification_events;
COMMIT;
