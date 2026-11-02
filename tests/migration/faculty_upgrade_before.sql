BEGIN;
INSERT INTO hostelhive.users(user_id,firebase_uid,email,role,is_active) VALUES
 ('34000000-0000-4000-8000-000000000001','faculty-upgrade-admin','faculty-upgrade-admin@example.invalid','admin',true),
 ('34000000-0000-4000-8000-000000000002','faculty-upgrade-one','faculty-upgrade-one@example.invalid','student',true),
 ('34000000-0000-4000-8000-000000000003','faculty-upgrade-two','faculty-upgrade-two@example.invalid','student',true),
 ('34000000-0000-4000-8000-000000000004','faculty-upgrade-archived','faculty-upgrade-archived@example.invalid','student',true);
INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES
 ('34000000-0000-4000-8000-000000000002','faculty-upgrade-one','Upgrade Student','Upgrade Science',1,'0770000000'),
 ('34000000-0000-4000-8000-000000000003','faculty-upgrade-two','Upgrade Student','UPGRADE SCIENCE',1,'0770000000'),
 ('34000000-0000-4000-8000-000000000004','faculty-upgrade-archived','Upgrade Student','Archived Faculty',1,'0770000000');
UPDATE hostelhive.students SET deleted_at=clock_timestamp(),deleted_by='34000000-0000-4000-8000-000000000001' WHERE index_no='faculty-upgrade-archived';
INSERT INTO hostelhive.announcements(request_id,published_by,title,content,audience,faculty) VALUES
 ('34000000-0000-4000-8000-000000000010','34000000-0000-4000-8000-000000000001','Historical targeted notice','Keep original audience','faculty','upgrade science'),
 ('34000000-0000-4000-8000-000000000011','34000000-0000-4000-8000-000000000001','Historical only','Keep original audience','faculty','historical only');
CREATE TABLE public.faculty_upgrade_snapshot AS SELECT
 (SELECT jsonb_agg(to_jsonb(s) ORDER BY student_id) FROM hostelhive.students s) AS students,
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY announcement_id) FROM hostelhive.announcements a) AS announcements,
 (SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox o) AS outbox;
COMMIT;
