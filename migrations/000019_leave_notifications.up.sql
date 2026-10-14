BEGIN;
CREATE TABLE hostelhive.notification_events (
 event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 source_type VARCHAR(16) NOT NULL CHECK(source_type IN ('complaint','leave')),
 complaint_update_id UUID UNIQUE REFERENCES hostelhive.complaint_updates(update_id) ON DELETE CASCADE,
 leave_id UUID REFERENCES hostelhive.leave_requests(leave_id) ON DELETE CASCADE,
 kind VARCHAR(24) NOT NULL,
 status VARCHAR(16) NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
 UNIQUE(leave_id,kind),
 CHECK((source_type='complaint' AND complaint_update_id IS NOT NULL AND leave_id IS NULL
        AND kind IN ('created','status') AND status IN ('pending','in_progress','resolved'))
    OR (source_type='leave' AND complaint_update_id IS NULL AND leave_id IS NOT NULL
        AND ((kind='leave_submitted' AND status='pending')
          OR (kind='leave_decided' AND status IN ('approved','rejected')))))
);
-- Preserve event IDs, notification IDs, delivered timestamps and read state.
-- Copy only existing complaint delivery intents; do not send historical alerts.
INSERT INTO hostelhive.notification_events(event_id,source_type,complaint_update_id,kind,status,occurred_at)
SELECT e.update_id,'complaint',e.update_id,e.action,e.new_status,e.occurred_at
FROM hostelhive.complaint_updates e
WHERE EXISTS(SELECT 1 FROM hostelhive.notification_outbox o WHERE o.event_id=e.update_id);
ALTER TABLE hostelhive.notification_outbox DROP CONSTRAINT notification_outbox_event_id_fkey;
ALTER TABLE hostelhive.notification_outbox ADD CONSTRAINT notification_outbox_event_id_fkey
 FOREIGN KEY(event_id) REFERENCES hostelhive.notification_events(event_id) ON DELETE CASCADE;
CREATE OR REPLACE FUNCTION hostelhive.enqueue_complaint_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c hostelhive.complaints%ROWTYPE;
BEGIN
 IF NEW.action NOT IN ('created','status') THEN RETURN NEW; END IF;
 SELECT * INTO STRICT c FROM hostelhive.complaints WHERE complaint_id=NEW.complaint_id;
 INSERT INTO hostelhive.notification_events(event_id,source_type,complaint_update_id,kind,status,occurred_at)
 VALUES(NEW.update_id,'complaint',NEW.update_id,NEW.action,NEW.new_status,NEW.occurred_at);
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
CREATE FUNCTION hostelhive.enqueue_leave_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid UUID; event_kind TEXT; event_time TIMESTAMPTZ;
BEGIN
 IF TG_OP='INSERT' THEN
  event_kind:='leave_submitted'; event_time:=NEW.created_at;
 ELSIF OLD.status='pending' AND NEW.status IN ('approved','rejected') THEN
  event_kind:='leave_decided'; event_time:=NEW.decided_at;
 ELSE
  RETURN NEW;
 END IF;
 INSERT INTO hostelhive.notification_events(source_type,leave_id,kind,status,occurred_at)
 VALUES('leave',NEW.leave_id,event_kind,NEW.status,event_time) RETURNING event_id INTO eid;
 IF event_kind='leave_submitted' THEN
  INSERT INTO hostelhive.notification_outbox(event_id,recipient_id)
  SELECT eid,u.user_id FROM hostelhive.users u WHERE u.is_active AND u.role IN ('admin','warden')
  ON CONFLICT DO NOTHING;
 ELSE
  INSERT INTO hostelhive.notification_outbox(event_id,recipient_id)
  SELECT eid,s.user_id FROM hostelhive.students s WHERE s.student_id=NEW.student_id
  ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER leave_notifications AFTER INSERT OR UPDATE OF status ON hostelhive.leave_requests
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_leave_notifications();
-- Existing leave records are not backfilled. A future decision on an existing
-- pending leave still generates its one decision event.
COMMIT;
