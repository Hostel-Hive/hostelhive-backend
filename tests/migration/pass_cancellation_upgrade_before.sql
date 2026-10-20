-- Disposable schema27 fixture, no live identities or provider calls.
INSERT INTO hostelhive.users(user_id,firebase_uid,email,role,is_active) VALUES
 ('10100000-0000-4000-8000-000000000001','upgrade101-admin','upgrade101-admin@example.invalid','admin',false),
 ('10100000-0000-4000-8000-000000000002','upgrade101-student','upgrade101-student@example.invalid','student',true);
INSERT INTO hostelhive.students(student_id,user_id,index_no,full_name,faculty,year,contact_phone)
 VALUES('10100000-0000-4000-8000-000000000003','10100000-0000-4000-8000-000000000002','UPGRADE101','Legacy cancellation Student','Science',1,'0771234567');
INSERT INTO hostelhive.leave_requests(leave_id,request_id,student_id,submitted_by,starts_at,ends_at,reason)
 SELECT ('10100000-0000-4000-8000-00000000001'||n)::uuid,gen_random_uuid(),'10100000-0000-4000-8000-000000000003','10100000-0000-4000-8000-000000000002','2040-01-01','2040-01-02','Upgrade fixture' FROM generate_series(1,3) n;
UPDATE hostelhive.leave_requests SET status='approved',decided_at=clock_timestamp(),decided_by='10100000-0000-4000-8000-000000000001' WHERE leave_id='10100000-0000-4000-8000-000000000012';
UPDATE hostelhive.leave_requests SET status='rejected',decision_note='No',decided_at=clock_timestamp(),decided_by='10100000-0000-4000-8000-000000000001' WHERE leave_id='10100000-0000-4000-8000-000000000013';
INSERT INTO hostelhive.late_access_requests(late_access_id,request_id,student_id,submitted_by,direction,expected_at,reason)
 SELECT ('10100000-0000-4000-8000-00000000002'||n)::uuid,gen_random_uuid(),'10100000-0000-4000-8000-000000000003','10100000-0000-4000-8000-000000000002','IN','2040-01-01'::timestamptz+n*interval '1 hour','Upgrade fixture' FROM generate_series(1,3) n;
UPDATE hostelhive.late_access_requests SET status='approved',decided_at=clock_timestamp(),decided_by='10100000-0000-4000-8000-000000000001' WHERE late_access_id='10100000-0000-4000-8000-000000000022';
UPDATE hostelhive.late_access_requests SET status='rejected',decision_note='No',decided_at=clock_timestamp(),decided_by='10100000-0000-4000-8000-000000000001' WHERE late_access_id='10100000-0000-4000-8000-000000000023';
INSERT INTO hostelhive.scanner_devices(device_id,name,credential_hash,direction,created_by,updated_by)
 VALUES('10100000-0000-4000-8000-000000000004','upgrade101-gate',decode(repeat('00',32),'hex'),'IN','10100000-0000-4000-8000-000000000001','10100000-0000-4000-8000-000000000001');
INSERT INTO hostelhive.attendance_events(event_id,device_id,student_id,payload_hash,direction,outcome,occurred_at,access_result,late_access_id)
 VALUES('10100000-0000-4000-8000-000000000005','10100000-0000-4000-8000-000000000004','10100000-0000-4000-8000-000000000003',decode(repeat('00',32),'hex'),'IN','recorded','2040-01-01T02:00:00Z','approved','10100000-0000-4000-8000-000000000022');
UPDATE hostelhive.late_access_requests SET consumed_at='2040-01-01T02:00:00Z',consumed_device_id='10100000-0000-4000-8000-000000000004',consumed_event_id='10100000-0000-4000-8000-000000000005' WHERE late_access_id='10100000-0000-4000-8000-000000000022';
INSERT INTO hostelhive.student_presence(student_id,status,updated_at,device_id,event_id)
 SELECT student_id,'IN',occurred_at,device_id,event_id FROM hostelhive.attendance_events WHERE event_id='10100000-0000-4000-8000-000000000005';
INSERT INTO hostelhive.notifications(event_id,recipient_id,read_at)
 SELECT o.event_id,o.recipient_id,clock_timestamp() FROM hostelhive.notification_outbox o WHERE o.recipient_id='10100000-0000-4000-8000-000000000002';
UPDATE hostelhive.notification_outbox SET delivered_at=clock_timestamp() WHERE recipient_id='10100000-0000-4000-8000-000000000002';
CREATE TABLE public.pass_cancellation_upgrade_snapshot(value JSONB NOT NULL);
INSERT INTO public.pass_cancellation_upgrade_snapshot SELECT jsonb_build_object(
 'leaves',(SELECT jsonb_agg(to_jsonb(v)-'cancelled_at'-'cancelled_by' ORDER BY leave_id) FROM hostelhive.leave_requests v),
 'late',(SELECT jsonb_agg(to_jsonb(v)-'cancelled_at'-'cancelled_by' ORDER BY late_access_id) FROM hostelhive.late_access_requests v),
 'events',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id) FROM hostelhive.notification_events v),
 'outbox',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox v),
 'inbox',(SELECT jsonb_agg(to_jsonb(v) ORDER BY notification_id) FROM hostelhive.notifications v),
  'attendance',(SELECT jsonb_agg(to_jsonb(v) ORDER BY device_id,event_id) FROM hostelhive.attendance_events v),
 'presence',(SELECT jsonb_agg(to_jsonb(v) ORDER BY student_id) FROM hostelhive.student_presence v),
 'realtime',(SELECT to_jsonb(v) FROM hostelhive.attendance_publication v));
