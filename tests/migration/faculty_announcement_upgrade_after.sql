BEGIN;
DO $$
DECLARE b RECORD; actual JSONB; au UUID; aid UUID; invalid BOOLEAN;
BEGIN
 FOR b IN SELECT * FROM public.faculty_announcement_upgrade_evidence LOOP
  CASE b.kind
   WHEN 'announcement' THEN SELECT to_jsonb(a)-'faculty' INTO actual FROM hostelhive.announcements a WHERE a.announcement_id::text=b.id;
   WHEN 'event' THEN SELECT to_jsonb(e) INTO actual FROM hostelhive.notification_events e WHERE e.event_id::text=b.id;
   WHEN 'outbox' THEN SELECT to_jsonb(o) INTO actual FROM hostelhive.notification_outbox o WHERE o.event_id::text||':'||o.recipient_id::text=b.id;
   WHEN 'notification' THEN SELECT to_jsonb(n) INTO actual FROM hostelhive.notifications n WHERE n.notification_id::text=b.id;
  END CASE;
  IF actual IS DISTINCT FROM b.data THEN RAISE EXCEPTION 'Faculty migration changed existing announcement or delivery history: %',b.kind; END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM hostelhive.announcements WHERE title='Legacy broadcast' AND (audience<>'all_students' OR faculty IS NOT NULL)) THEN RAISE EXCEPTION 'Legacy broadcast target changed'; END IF;
 SELECT user_id INTO STRICT au FROM hostelhive.users WHERE firebase_uid='faculty-upgrade-admin';
 INSERT INTO hostelhive.announcements(request_id,published_by,title,content,audience,faculty) VALUES(gen_random_uuid(),au,'Targeted upgrade notice','Science only','faculty','science') RETURNING announcement_id INTO aid;
 IF (SELECT count(*) FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e USING(event_id) WHERE e.announcement_id=aid)<>1 THEN RAISE EXCEPTION 'Incorrect faculty recipient count'; END IF;
 IF EXISTS(SELECT 1 FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e USING(event_id) JOIN hostelhive.users u ON u.user_id=o.recipient_id WHERE e.announcement_id=aid AND u.firebase_uid<>'faculty-upgrade-science') THEN RAISE EXCEPTION 'Cross-faculty recipient selected'; END IF;
 invalid:=false;
 BEGIN
  INSERT INTO hostelhive.announcements(request_id,published_by,title,content,audience) VALUES(gen_random_uuid(),au,'Invalid','No faculty','faculty');
 EXCEPTION WHEN check_violation THEN invalid:=true;
 END;
 IF NOT invalid THEN RAISE EXCEPTION 'Missing faculty accepted'; END IF;
 invalid:=false;
 BEGIN
  INSERT INTO hostelhive.announcements(request_id,published_by,title,content,audience,faculty) VALUES(gen_random_uuid(),au,'Invalid','Conflicting target','all_students','science');
 EXCEPTION WHEN check_violation THEN invalid:=true;
 END;
 IF NOT invalid THEN RAISE EXCEPTION 'Conflicting broadcast faculty accepted'; END IF;
END $$;
COMMIT;
