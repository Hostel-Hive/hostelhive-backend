-- Run at schema25 on a disposable database only.
INSERT INTO hostelhive.users(user_id,firebase_uid,email,role,is_active) VALUES
 ('95000000-0000-4000-8000-000000000001','migration95-legacy','migration95-legacy@example.invalid','student',true),
 ('95000000-0000-4000-8000-000000000002','migration95-admin','migration95-admin@example.invalid','admin',false);
INSERT INTO hostelhive.students(student_id,user_id,index_no,full_name,faculty,year,contact_phone)
 VALUES('95000000-0000-4000-8000-000000000011','95000000-0000-4000-8000-000000000001','MIGRATION-95-LEGACY','Legacy Student','Science',1,'0771234567');
INSERT INTO hostelhive.guardians(student_id,name,relationship,contact_phone)
 VALUES('95000000-0000-4000-8000-000000000011','Guardian','Parent','0771234568');
CREATE TABLE public.student_invitation_upgrade_snapshot(value JSONB NOT NULL);
INSERT INTO public.student_invitation_upgrade_snapshot SELECT jsonb_build_object(
 'users',(SELECT jsonb_agg(to_jsonb(u) ORDER BY user_id) FROM hostelhive.users u),
 'students',(SELECT jsonb_agg(to_jsonb(s) ORDER BY student_id) FROM hostelhive.students s),
 'guardians',(SELECT jsonb_agg(to_jsonb(g) ORDER BY guardian_id) FROM hostelhive.guardians g),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY device_id,event_id) FROM hostelhive.attendance_events e),
 'presence',(SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p),
 'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n));
