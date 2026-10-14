BEGIN;
CREATE TABLE hostelhive.notification_outbox (
 event_id UUID NOT NULL REFERENCES hostelhive.complaint_updates(update_id) ON DELETE CASCADE,
 recipient_id UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE CASCADE,
 delivered_at TIMESTAMPTZ,
 PRIMARY KEY(event_id,recipient_id)
);
CREATE INDEX notification_outbox_pending ON hostelhive.notification_outbox(event_id,recipient_id) WHERE delivered_at IS NULL;
CREATE TABLE hostelhive.notifications (
 notification_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id UUID NOT NULL,
 recipient_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 read_at TIMESTAMPTZ,
 UNIQUE(event_id,recipient_id),
 FOREIGN KEY(event_id,recipient_id) REFERENCES hostelhive.notification_outbox(event_id,recipient_id) ON DELETE CASCADE
);
CREATE INDEX notifications_inbox ON hostelhive.notifications(recipient_id,created_at DESC,notification_id DESC);
-- Delivery intent commits atomically with each NEW workflow event. Retries do
-- not insert another complaint_update and therefore cannot create duplicates.
CREATE FUNCTION hostelhive.enqueue_complaint_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER complaint_notifications AFTER INSERT ON hostelhive.complaint_updates
FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_complaint_notifications();
-- No historical backfill: pre-migration complaints do not generate new alerts.
COMMIT;
