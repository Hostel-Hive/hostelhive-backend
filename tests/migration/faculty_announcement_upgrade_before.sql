-- Run at version28 on an isolated database.
BEGIN;
CREATE TABLE public.faculty_announcement_upgrade_evidence(kind TEXT,id TEXT,data JSONB);
DO $$
DECLARE au UUID; su UUID; ar UUID; aid UUID;
BEGIN
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('faculty-upgrade-admin','faculty-upgrade-admin@example.invalid','admin',true) RETURNING user_id INTO au;
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('faculty-upgrade-science','faculty-upgrade-science@example.invalid','student',true) RETURNING user_id INTO su;
 INSERT INTO hostelhive.users(firebase_uid,email,role,is_active) VALUES('faculty-upgrade-arts','faculty-upgrade-arts@example.invalid','student',true) RETURNING user_id INTO ar;
 INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES(su,'FACULTY-UPGRADE-SCIENCE','Science Student','SCIENCE',1,'0771234567'),(ar,'FACULTY-UPGRADE-ARTS','Arts Student','Arts',1,'0771234567');
 INSERT INTO hostelhive.announcements(request_id,published_by,title,content) VALUES(gen_random_uuid(),au,'Legacy broadcast','Retain the audience and receipts') RETURNING announcement_id INTO aid;
 INSERT INTO hostelhive.notifications(event_id,recipient_id,read_at)
 SELECT e.event_id,su,clock_timestamp() FROM hostelhive.notification_events e WHERE e.announcement_id=aid;
 UPDATE hostelhive.notification_outbox SET delivered_at=clock_timestamp() WHERE recipient_id=su AND event_id IN(SELECT event_id FROM hostelhive.notification_events WHERE announcement_id=aid);
 INSERT INTO public.faculty_announcement_upgrade_evidence SELECT 'announcement',a.announcement_id::text,to_jsonb(a) FROM hostelhive.announcements a WHERE a.announcement_id=aid;
 INSERT INTO public.faculty_announcement_upgrade_evidence SELECT 'event',e.event_id::text,to_jsonb(e) FROM hostelhive.notification_events e WHERE e.announcement_id=aid;
 INSERT INTO public.faculty_announcement_upgrade_evidence SELECT 'outbox',o.event_id::text||':'||o.recipient_id::text,to_jsonb(o) FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e USING(event_id) WHERE e.announcement_id=aid;
 INSERT INTO public.faculty_announcement_upgrade_evidence SELECT 'notification',n.notification_id::text,to_jsonb(n) FROM hostelhive.notifications n JOIN hostelhive.notification_events e USING(event_id) WHERE e.announcement_id=aid;
END $$;
COMMIT;
