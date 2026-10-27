BEGIN;
DELETE FROM hostelhive.announcements WHERE published_by IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid='faculty-upgrade-admin');
DELETE FROM hostelhive.students WHERE index_no LIKE 'FACULTY-UPGRADE-%';
DELETE FROM hostelhive.users WHERE firebase_uid IN('faculty-upgrade-admin','faculty-upgrade-science','faculty-upgrade-arts');
DROP TABLE public.faculty_announcement_upgrade_evidence;
COMMIT;
