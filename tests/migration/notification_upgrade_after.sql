-- Verify migration 18->19 preserves read, delivered and pending complaint state.
BEGIN;
DO $$
DECLARE b RECORD; actual JSONB;
BEGIN
 FOR b IN SELECT * FROM public.notification_upgrade_evidence LOOP
  IF b.kind='notification' THEN
   SELECT to_jsonb(n) INTO actual FROM hostelhive.notifications n WHERE n.notification_id::text=b.record_id;
  ELSE
   SELECT to_jsonb(o) INTO actual FROM hostelhive.notification_outbox o WHERE o.event_id::text||':'||o.recipient_id::text=b.record_id;
  END IF;
  IF actual IS DISTINCT FROM b.data THEN RAISE EXCEPTION 'Complaint notification state changed on upgrade'; END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM hostelhive.notification_events e JOIN hostelhive.leave_requests l ON l.leave_id=e.leave_id JOIN hostelhive.students s ON s.student_id=l.student_id WHERE s.index_no='UPGRADE-70') THEN
  RAISE EXCEPTION 'Upgrade backfilled old leave alerts';
 END IF;
 IF EXISTS(SELECT 1 FROM hostelhive.notification_outbox o LEFT JOIN hostelhive.notification_events e ON e.event_id=o.event_id WHERE e.event_id IS NULL) THEN
  RAISE EXCEPTION 'Complaint intent has no shared event';
 END IF;
END;
$$;
-- Remove only the dedicated upgrade fixture so subsequent integration tests are isolated.
DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-70'));
DELETE FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-70');
DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-70');
DELETE FROM hostelhive.students WHERE index_no='UPGRADE-70';
DELETE FROM hostelhive.users WHERE firebase_uid IN('upgrade-70-student','upgrade-70-warden');
DROP TABLE public.notification_upgrade_evidence;
COMMIT;
