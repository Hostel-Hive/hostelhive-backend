BEGIN;
DO $$
DECLARE b RECORD; actual JSONB;
BEGIN
 FOR b IN SELECT * FROM public.late_access_upgrade_evidence LOOP
  IF b.kind='notification' THEN
   SELECT to_jsonb(n) INTO actual FROM hostelhive.notifications n WHERE n.notification_id::text=b.record_id;
  ELSE
   SELECT to_jsonb(o) INTO actual FROM hostelhive.notification_outbox o WHERE o.event_id::text||':'||o.recipient_id::text=b.record_id;
  END IF;
  IF actual IS DISTINCT FROM b.data THEN RAISE EXCEPTION 'Mixed inbox state changed on late-access upgrade'; END IF;
 END LOOP;
END;
$$;
DELETE FROM hostelhive.announcements WHERE published_by IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid IN('upgrade-74-admin','upgrade-74-warden'));
DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-74'));
DELETE FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-74');
DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-74');
DELETE FROM hostelhive.students WHERE index_no='UPGRADE-74';
DELETE FROM hostelhive.users WHERE firebase_uid IN('upgrade-74-student','upgrade-74-admin','upgrade-74-warden');
DROP TABLE public.late_access_upgrade_evidence;
COMMIT;
