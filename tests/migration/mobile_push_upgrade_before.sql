-- Disposable schema24 only. Preserve an existing read inbox item and pending intent.
BEGIN;
CREATE TABLE public.push_upgrade_evidence(kind TEXT,record_id TEXT,data JSONB);
DO $$
DECLARE su UUID;au UUID;sid UUID;lid UUID;
BEGIN
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('upgrade89-student','upgrade89-student@example.invalid','student',true) RETURNING user_id INTO su;
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('upgrade89-admin','upgrade89-admin@example.invalid','admin',true) RETURNING user_id INTO au;
 INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES(su,'UPGRADE89','Upgrade Student','Science',1,'0771234567') RETURNING student_id INTO sid;
 INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason) VALUES(gen_random_uuid(),sid,su,clock_timestamp()+interval '2 days',clock_timestamp()+interval '3 days','Existing private reason') RETURNING leave_id INTO lid;
 UPDATE hostelhive.leave_requests SET status='approved',decided_at=clock_timestamp(),decided_by=au WHERE leave_id=lid;
 INSERT INTO hostelhive.notifications(event_id,recipient_id,read_at)
 SELECT o.event_id,o.recipient_id,clock_timestamp() FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e USING(event_id) WHERE e.leave_id=lid AND e.kind='leave_submitted';
 UPDATE hostelhive.notification_outbox o SET delivered_at=clock_timestamp() WHERE EXISTS(SELECT 1 FROM hostelhive.notifications n WHERE n.event_id=o.event_id AND n.recipient_id=o.recipient_id);
 INSERT INTO public.push_upgrade_evidence SELECT 'notification',n.notification_id::text,to_jsonb(n) FROM hostelhive.notifications n WHERE n.event_id IN(SELECT event_id FROM hostelhive.notification_events WHERE leave_id=lid);
 INSERT INTO public.push_upgrade_evidence SELECT 'outbox',o.event_id::text||':'||o.recipient_id::text,to_jsonb(o) FROM hostelhive.notification_outbox o WHERE o.event_id IN(SELECT event_id FROM hostelhive.notification_events WHERE leave_id=lid);
 INSERT INTO public.push_upgrade_evidence SELECT 'leave',l.leave_id::text,to_jsonb(l) FROM hostelhive.leave_requests l WHERE l.leave_id=lid;
END;
$$;
COMMIT;
