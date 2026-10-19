-- Disposable schema26 only.
INSERT INTO hostelhive.users(user_id,firebase_uid,email,role,is_active) VALUES
 ('10000000-0000-4000-8000-000000000001','upgrade100-admin','upgrade100-admin@example.invalid','admin',false),
 ('10000000-0000-4000-8000-000000000002','upgrade100-student','upgrade100-student@example.invalid','student',true);
INSERT INTO hostelhive.students(student_id,user_id,index_no,full_name,faculty,year,contact_phone)
 VALUES('10000000-0000-4000-8000-000000000003','10000000-0000-4000-8000-000000000002','UPGRADE100','Legacy Student','Science',1,'0771234567');
INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,direction,created_by,updated_by)
 VALUES('10000000-0000-4000-8000-000000000004','upgrade100-gate',decode(repeat('00',32),'hex'),'IN','10000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001');
INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at)
 VALUES('10000000-0000-4000-8000-000000000005','10000000-0000-4000-8000-000000000004','10000000-0000-4000-8000-000000000003',decode(repeat('00',32),'hex'),'IN','recorded',clock_timestamp()-interval '1 day');
INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id)
 SELECT student_id,'IN',occurred_at,device_id,event_id FROM hostelhive.attendance_events WHERE event_id='10000000-0000-4000-8000-000000000005';
CREATE TABLE public.going_home_upgrade_snapshot(value JSONB NOT NULL);
INSERT INTO public.going_home_upgrade_snapshot SELECT jsonb_build_object(
 'users',(SELECT jsonb_agg(to_jsonb(u) ORDER BY user_id) FROM hostelhive.users u),
 'students',(SELECT jsonb_agg(to_jsonb(s) ORDER BY student_id) FROM hostelhive.students s),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY device_id,event_id) FROM hostelhive.attendance_events e),
 'presence',(SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p),
 'publication',(SELECT to_jsonb(p) FROM hostelhive.attendance_publication p),
 'invitations',(SELECT jsonb_agg(to_jsonb(i) ORDER BY invitation_id) FROM hostelhive.student_invitations i),
 'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n));
