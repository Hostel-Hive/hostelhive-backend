BEGIN;
DROP TRIGGER going_home_departure_recorded ON hostelhive.attendance_events;
DROP FUNCTION hostelhive.going_home_record_departure();
DROP TABLE hostelhive.going_home_declarations;
COMMIT;
