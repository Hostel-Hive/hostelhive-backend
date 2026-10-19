BEGIN;
CREATE TABLE hostelhive.going_home_declarations (
 going_home_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 request_id UUID NOT NULL CHECK(request_id<>'00000000-0000-0000-0000-000000000000'),
 student_id UUID NOT NULL,
 submitted_by UUID NOT NULL,
 status VARCHAR(8) NOT NULL DEFAULT 'declared' CHECK(status IN ('declared','departed')),
 declared_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 departed_at TIMESTAMPTZ,
 departure_device_id UUID,
 departure_event_id UUID,
 UNIQUE(student_id,request_id),
 FOREIGN KEY(student_id,submitted_by) REFERENCES hostelhive.students(student_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(departure_device_id,departure_event_id) REFERENCES hostelhive.attendance_events(device_id,event_id) ON DELETE RESTRICT,
 CHECK((status='declared' AND departed_at IS NULL AND departure_device_id IS NULL AND departure_event_id IS NULL)
    OR (status='departed' AND departed_at IS NOT NULL AND departed_at>=declared_at AND departure_device_id IS NOT NULL AND departure_event_id IS NOT NULL))
);
CREATE UNIQUE INDEX going_home_one_outstanding ON hostelhive.going_home_declarations(student_id) WHERE status='declared';
CREATE INDEX going_home_student_history ON hostelhive.going_home_declarations(student_id,declared_at DESC,going_home_id DESC);
-- Consume intent only after an actual accepted OUT event. Never write attendance.
CREATE FUNCTION hostelhive.going_home_record_departure() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM 1 FROM hostelhive.students WHERE student_id=NEW.student_id FOR UPDATE;
 UPDATE hostelhive.going_home_declarations
 SET status='departed',departed_at=NEW.occurred_at,departure_device_id=NEW.device_id,departure_event_id=NEW.event_id
 WHERE student_id=NEW.student_id AND status='declared' AND declared_at<=NEW.occurred_at;
 RETURN NEW;
END;
$$;
CREATE TRIGGER going_home_departure_recorded AFTER INSERT ON hostelhive.attendance_events
 FOR EACH ROW WHEN (NEW.outcome='recorded' AND NEW.direction='OUT')
 EXECUTE FUNCTION hostelhive.going_home_record_departure();
COMMIT;
