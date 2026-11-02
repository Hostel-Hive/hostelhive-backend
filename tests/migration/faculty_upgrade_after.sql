BEGIN;
DO $$ DECLARE snapshot RECORD; BEGIN
 SELECT * INTO snapshot FROM public.faculty_upgrade_snapshot;
 IF snapshot.students IS DISTINCT FROM (SELECT jsonb_agg(to_jsonb(s) ORDER BY student_id) FROM hostelhive.students s)
 OR snapshot.announcements IS DISTINCT FROM (SELECT jsonb_agg(to_jsonb(a) ORDER BY announcement_id) FROM hostelhive.announcements a)
 OR snapshot.outbox IS DISTINCT FROM (SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox o) THEN
  RAISE EXCEPTION 'Faculty upgrade changed existing students, publications or delivery state';
 END IF;
 IF (SELECT count(*) FROM hostelhive.faculties WHERE lower(name)='upgrade science')<>1 THEN
  RAISE EXCEPTION 'Faculty upgrade failed to merge case variants';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM hostelhive.faculty_names WHERE name_key='archived faculty')
 OR NOT EXISTS(SELECT 1 FROM hostelhive.faculty_names WHERE name_key='historical only') THEN
  RAISE EXCEPTION 'Faculty upgrade lost archived or historical audiences';
 END IF;
END $$;
DROP TABLE public.faculty_upgrade_snapshot;
DELETE FROM hostelhive.announcements WHERE published_by='34000000-0000-4000-8000-000000000001';
DELETE FROM hostelhive.students WHERE index_no LIKE 'faculty-upgrade-%';
DELETE FROM hostelhive.users WHERE firebase_uid LIKE 'faculty-upgrade-%';
DELETE FROM hostelhive.faculties WHERE lower(name) IN ('upgrade science','archived faculty','historical only');
COMMIT;
