BEGIN;
DO $$
DECLARE evidence record; preserved boolean; sid uuid; device uuid; event uuid;
BEGIN
 FOR evidence IN SELECT * FROM public.hostel_policy_upgrade_evidence LOOP
  EXECUTE format('SELECT EXISTS(SELECT 1 FROM hostelhive.%I v WHERE to_jsonb(v)=$1)',evidence.source_table) INTO preserved USING evidence.payload;
  IF NOT preserved THEN RAISE EXCEPTION 'Policy upgrade changed historical %',evidence.source_table; END IF;
 END LOOP;
 SELECT student_id INTO sid FROM hostelhive.students WHERE index_no='policy118-upgrade';
 SELECT device_id INTO device FROM hostelhive.scanner_devices WHERE name='policy118-upgrade';
 event := gen_random_uuid();
 INSERT INTO hostelhive.attendance_events(device_id,event_id,student_id,payload_hash,direction,outcome,occurred_at,access_result) VALUES(device,event,sid,decode(repeat('06',32),'hex'),'OUT','recorded',clock_timestamp(),'unapproved');
 INSERT INTO hostelhive.attendance_incidents(device_id,event_id,student_id,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds) VALUES(device,event,sid,'OUT','late_exit',clock_timestamp(),'Asia/Colombo',360,1200,1800);
 BEGIN
  INSERT INTO hostelhive.attendance_incidents(device_id,event_id,student_id,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds) VALUES(device,gen_random_uuid(),sid,'IN','late_exit',clock_timestamp(),'Asia/Colombo',360,1200,1800);
  RAISE EXCEPTION 'Accepted mismatched incident direction';
 EXCEPTION WHEN check_violation THEN NULL;
 END;
END $$;
COMMIT;
