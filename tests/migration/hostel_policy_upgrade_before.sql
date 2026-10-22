BEGIN;
CREATE TABLE public.hostel_policy_upgrade_evidence (source_table text NOT NULL, payload jsonb NOT NULL);
DO $$
DECLARE owner_id uuid; student_user uuid; sid uuid; device uuid; event uuid; tbl text;
BEGIN
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('policy118-admin','policy118-admin@example.invalid','admin',true) RETURNING user_id INTO owner_id;
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('policy118-student','policy118-student@example.invalid','student',true) RETURNING user_id INTO student_user;
 INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES(student_user,'policy118-upgrade','Policy upgrade','Science',4,'0771234567') RETURNING student_id INTO sid;
 INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason) VALUES(gen_random_uuid(),sid,student_user,clock_timestamp()+interval '1 day',clock_timestamp()+interval '2 days','Historical leave');
 INSERT INTO hostelhive.late_access_requests(request_id,student_id,submitted_by,direction,expected_at,reason) VALUES(gen_random_uuid(),sid,student_user,'OUT',clock_timestamp()+interval '1 day','Historical approval');
 INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,direction,created_by,updated_by) VALUES(gen_random_uuid(),'policy118-upgrade',decode(repeat('04',32),'hex'),'OUT',owner_id,owner_id) RETURNING device_id INTO device;
 event := gen_random_uuid();
 INSERT INTO hostelhive.attendance_events(device_id,event_id,student_id,payload_hash,direction,outcome,occurred_at) VALUES(device,event,sid,decode(repeat('05',32),'hex'),'OUT','recorded',clock_timestamp());
 INSERT INTO hostelhive.attendance_incidents(device_id,event_id,student_id,direction,incident_type,occurred_at,hostel_timezone,exit_opening_minute,entry_deadline_minute,permission_window_seconds) VALUES(device,event,sid,'OUT','early_exit',clock_timestamp(),'Asia/Colombo',360,1200,1800);
 FOREACH tbl IN ARRAY ARRAY['leave_requests','late_access_requests','going_home_declarations','attendance_events','attendance_incidents','student_presence','notification_events','notification_outbox','notifications'] LOOP
  EXECUTE format('INSERT INTO public.hostel_policy_upgrade_evidence SELECT %L,to_jsonb(v) FROM hostelhive.%I v',tbl,tbl);
 END LOOP;
END $$;
COMMIT;
