-- Run on disposable schema 20. Preserve a mixed complaint/leave inbox.
BEGIN;
CREATE TABLE public.late_access_upgrade_evidence(kind TEXT,record_id TEXT,data JSONB);
DO $$
DECLARE su UUID; au UUID; wu UUID; sid UUID; cid UUID; lid UUID;
BEGIN
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('upgrade-74-student','upgrade-74-student@example.invalid','student',true) RETURNING user_id INTO su;
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('upgrade-74-admin','upgrade-74-admin@example.invalid','admin',true) RETURNING user_id INTO au;
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('upgrade-74-warden','upgrade-74-warden@example.invalid','warden',true) RETURNING user_id INTO wu;
 INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES(su,'UPGRADE-74','Upgrade Student','Engineering',1,'0771234567') RETURNING student_id INTO sid;
 INSERT INTO hostelhive.leave_requests(request_id,student_id,submitted_by,starts_at,ends_at,reason) VALUES(gen_random_uuid(),sid,su,clock_timestamp()+interval '2 days',clock_timestamp()+interval '3 days','Existing leave') RETURNING leave_id INTO lid;
 UPDATE hostelhive.leave_requests SET status='approved',decided_at=clock_timestamp(),decided_by=wu WHERE leave_id=lid;
 INSERT INTO hostelhive.complaints(request_id,student_id,submitted_by,category,description) VALUES(gen_random_uuid(),sid,su,'maintenance','Existing complaint') RETURNING complaint_id INTO cid;
 INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,new_status,previous_version,version) VALUES(cid,gen_random_uuid(),su,'created','pending',0,1);
 INSERT INTO hostelhive.notifications(event_id,recipient_id,read_at)
 SELECT o.event_id,o.recipient_id,CASE WHEN e.source_type='complaint' THEN clock_timestamp() ELSE NULL END
 FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e ON e.event_id=o.event_id
 WHERE (e.leave_id=lid OR e.complaint_update_id IN(SELECT update_id FROM hostelhive.complaint_updates WHERE complaint_id=cid)) AND e.kind<>'leave_decided';
 UPDATE hostelhive.notification_outbox o SET delivered_at=clock_timestamp()
 WHERE EXISTS(SELECT 1 FROM hostelhive.notifications n WHERE n.event_id=o.event_id AND n.recipient_id=o.recipient_id);
 INSERT INTO hostelhive.announcements(request_id,published_by,title,content) VALUES(gen_random_uuid(),au,'Existing notice','Preserve this notice');
 INSERT INTO hostelhive.notifications(event_id,recipient_id,read_at)
 SELECT o.event_id,o.recipient_id,clock_timestamp() FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e ON e.event_id=o.event_id
 WHERE e.announcement_id IN(SELECT announcement_id FROM hostelhive.announcements WHERE published_by=au);
 UPDATE hostelhive.notification_outbox o SET delivered_at=clock_timestamp()
 WHERE EXISTS(SELECT 1 FROM hostelhive.notifications n WHERE n.event_id=o.event_id AND n.recipient_id=o.recipient_id);
 INSERT INTO public.late_access_upgrade_evidence SELECT 'notification',n.notification_id::text,to_jsonb(n) FROM hostelhive.notifications n JOIN hostelhive.users u ON u.user_id=n.recipient_id WHERE u.firebase_uid IN('upgrade-74-student','upgrade-74-admin','upgrade-74-warden');
 INSERT INTO public.late_access_upgrade_evidence SELECT 'outbox',o.event_id::text||':'||o.recipient_id::text,to_jsonb(o) FROM hostelhive.notification_outbox o JOIN hostelhive.users u ON u.user_id=o.recipient_id WHERE u.firebase_uid IN('upgrade-74-student','upgrade-74-admin','upgrade-74-warden');
END;
$$;
COMMIT;
