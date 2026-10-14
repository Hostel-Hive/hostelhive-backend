BEGIN;
CREATE TABLE hostelhive.late_access_requests (
 late_access_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id UUID NOT NULL,
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 submitted_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 direction VARCHAR(3) NOT NULL CHECK(direction IN ('IN','OUT')),
 expected_at TIMESTAMPTZ NOT NULL CHECK(isfinite(expected_at)),
 reason VARCHAR(1000) NOT NULL CHECK(reason=btrim(reason) AND length(reason)>0),
 status VARCHAR(8) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 decided_at TIMESTAMPTZ,
 decided_by UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 decision_note VARCHAR(1000) NOT NULL DEFAULT '',
 UNIQUE(student_id,request_id),
 CHECK((status='pending' AND decided_at IS NULL AND decided_by IS NULL AND decision_note='') OR
       (status IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL)),
 CHECK(status<>'rejected' OR length(btrim(decision_note))>0)
);
CREATE INDEX late_access_requests_student_history ON hostelhive.late_access_requests(student_id,created_at DESC,late_access_id DESC);
CREATE INDEX late_access_requests_pending ON hostelhive.late_access_requests(created_at,late_access_id) WHERE status='pending';

CREATE UNIQUE INDEX late_access_requests_active_movement ON hostelhive.late_access_requests(student_id,direction,expected_at) WHERE status IN ('pending','approved');
ALTER TABLE hostelhive.notification_events ADD COLUMN late_access_id UUID REFERENCES hostelhive.late_access_requests(late_access_id) ON DELETE CASCADE;
ALTER TABLE hostelhive.notification_events ADD CONSTRAINT notification_events_late_access_kind UNIQUE(late_access_id,kind);
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_source_type_check;
ALTER TABLE hostelhive.notification_events DROP CONSTRAINT notification_events_check;
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
CREATE FUNCTION hostelhive.enqueue_late_access_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid UUID; event_kind TEXT; event_time TIMESTAMPTZ;
BEGIN
 IF TG_OP='INSERT' THEN
  event_kind:='late_access_submitted'; event_time:=NEW.created_at;
 ELSIF OLD.status='pending' AND NEW.status IN ('approved','rejected') THEN
  event_kind:='late_access_decided'; event_time:=NEW.decided_at;
 ELSE
  RETURN NEW;
 END IF;
 INSERT INTO hostelhive.notification_events(source_type,late_access_id,kind,status,occurred_at)
 VALUES('late_access',NEW.late_access_id,event_kind,NEW.status,event_time) RETURNING event_id INTO eid;
 IF event_kind='late_access_submitted' THEN
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
CREATE TRIGGER late_access_notifications AFTER INSERT OR UPDATE OF status ON hostelhive.late_access_requests
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_late_access_notifications();
COMMIT;