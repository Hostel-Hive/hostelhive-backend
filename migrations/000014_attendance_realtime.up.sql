BEGIN;
-- A coalescing transactional outbox: every relevant commit makes the latest
-- complete projection pending. Attendance history remains in attendance_events.
CREATE TABLE hostelhive.attendance_publication (
 singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),
 version BIGINT NOT NULL,
 changed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 pending BOOLEAN NOT NULL DEFAULT true,
 attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 published_at TIMESTAMPTZ,
 published_version BIGINT
);
INSERT INTO hostelhive.attendance_publication(singleton,version)
VALUES(true,(extract(epoch FROM clock_timestamp())*1000)::bigint);

CREATE TABLE hostelhive.realtime_staff_grants (
 firebase_uid TEXT PRIMARY KEY,
 version BIGINT NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 pending BOOLEAN NOT NULL DEFAULT true,
 attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE FUNCTION hostelhive.queue_attendance_publication() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 UPDATE hostelhive.attendance_publication
 SET version=greatest(version+1,(extract(epoch FROM clock_timestamp())*1000)::bigint),
     changed_at=clock_timestamp(),pending=true,next_attempt_at=clock_timestamp();
 RETURN NULL;
END;
$$;
CREATE TRIGGER presence_publication AFTER INSERT OR UPDATE OR DELETE ON hostelhive.student_presence
FOR EACH STATEMENT EXECUTE FUNCTION hostelhive.queue_attendance_publication();
CREATE TRIGGER student_publication AFTER INSERT OR UPDATE OR DELETE ON hostelhive.students
FOR EACH STATEMENT EXECUTE FUNCTION hostelhive.queue_attendance_publication();
CREATE TRIGGER account_publication AFTER INSERT OR UPDATE OR DELETE ON hostelhive.users
FOR EACH STATEMENT EXECUTE FUNCTION hostelhive.queue_attendance_publication();

CREATE FUNCTION hostelhive.invalidate_realtime_grant() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.firebase_uid IS DISTINCT FROM OLD.firebase_uid
    OR NOT NEW.is_active OR NEW.role NOT IN ('admin','warden') THEN
  UPDATE hostelhive.realtime_staff_grants
  SET version=greatest(version+1,(extract(epoch FROM clock_timestamp())*1000)::bigint),
      expires_at=to_timestamp(0),pending=true,next_attempt_at=clock_timestamp()
  WHERE firebase_uid=OLD.firebase_uid;
 END IF;
 RETURN NULL;
END;
$$;
CREATE TRIGGER account_realtime_grant AFTER UPDATE OR DELETE ON hostelhive.users
FOR EACH ROW EXECUTE FUNCTION hostelhive.invalidate_realtime_grant();
COMMIT;
