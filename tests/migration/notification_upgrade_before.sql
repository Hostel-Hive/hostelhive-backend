-- Run only on a disposable test database at migration 18.
BEGIN;
CREATE TABLE public.notification_upgrade_evidence(kind TEXT,record_id TEXT,data JSONB);
DO $$
DECLARE su UUID; wu UUID; sid UUID; cid UUID;
BEGIN
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('upgrade-70-student','upgrade-70-student@example.invalid','student',true) RETURNING user_id INTO su;
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('upgrade-70-warden','upgrade-70-warden@example.invalid','warden',true) RETURNING user_id INTO wu;
 INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES(su,'UPGRADE-70','Upgrade Test','Engineering',1,'0771234567') RETURNING student_id INTO sid;
 INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason) VALUES(gen_random_uuid(),sid,su,clock_timestamp()+interval '2 days',clock_timestamp()+interval '3 days','Existing pending leave');
 INSERT INTO hostelhive.complaints(request_id,student_id,submitted_by,category,description) VALUES(gen_random_uuid(),sid,su,'maintenance','Existing complaint') RETURNING complaint_id INTO cid;
 INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,new_status,previous_version,version) VALUES(cid,gen_random_uuid(),su,'created','pending',0,1);
 INSERT INTO hostelhive.notifications(event_id,recipient_id,read_at)
 SELECT o.event_id,o.recipient_id,clock_timestamp() FROM hostelhive.notification_outbox o JOIN hostelhive.complaint_updates e ON e.update_id=o.event_id WHERE e.complaint_id=cid;
 UPDATE hostelhive.notification_outbox o SET delivered_at=clock_timestamp() FROM hostelhive.complaint_updates e WHERE e.update_id=o.event_id AND e.complaint_id=cid;
 UPDATE hostelhive.complaints SET status='in_progress',version=2 WHERE complaint_id=cid;
 INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,previous_status,new_status,previous_version,version) VALUES(cid,gen_random_uuid(),wu,'status','pending','in_progress',1,2);
 INSERT INTO public.notification_upgrade_evidence SELECT 'notification',n.notification_id::text,to_jsonb(n) FROM hostelhive.notifications n JOIN hostelhive.complaint_updates e ON e.update_id=n.event_id WHERE e.complaint_id=cid;
 INSERT INTO public.notification_upgrade_evidence SELECT 'outbox',o.event_id::text||':'||o.recipient_id::text,to_jsonb(o) FROM hostelhive.notification_outbox o JOIN hostelhive.complaint_updates e ON e.update_id=o.event_id WHERE e.complaint_id=cid;
END;
$$;
COMMIT;
