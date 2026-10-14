BEGIN;
DO $$
DECLARE b RECORD; actual JSONB;
BEGIN
 FOR b IN SELECT * FROM public.attendance_permission_upgrade_evidence LOOP
  CASE b.kind
  WHEN 'notification' THEN SELECT to_jsonb(n) INTO actual FROM hostelhive.notifications n WHERE n.notification_id::text=b.record_id;
  WHEN 'outbox' THEN SELECT to_jsonb(o) INTO actual FROM hostelhive.notification_outbox o WHERE o.event_id::text||':'||o.recipient_id::text=b.record_id;
  WHEN 'approval' THEN SELECT to_jsonb(l)-'consumed_at'-'consumed_device_id'-'consumed_event_id' INTO actual FROM hostelhive.late_access_requests l WHERE l.late_access_id::text=b.record_id;
  WHEN 'scan' THEN SELECT to_jsonb(e)-'access_result'-'late_access_id' INTO actual FROM hostelhive.attendance_events e WHERE e.device_id::text||':'||e.event_id::text=b.record_id;
  WHEN 'presence' THEN SELECT to_jsonb(p) INTO actual FROM hostelhive.student_presence p WHERE p.student_id::text=b.record_id;
  ELSE RAISE EXCEPTION 'Unknown preservation evidence';
  END CASE;
  IF actual IS DISTINCT FROM b.data THEN RAISE EXCEPTION 'Existing state changed on permission upgrade: %',b.kind; END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM hostelhive.attendance_events WHERE access_result IS NOT NULL AND student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76')) THEN RAISE EXCEPTION 'Historical scans reclassified'; END IF;
 IF EXISTS(SELECT 1 FROM hostelhive.late_access_requests WHERE consumed_at IS NOT NULL AND student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76')) THEN RAISE EXCEPTION 'Historical approval consumed'; END IF;
END;
$$;
DELETE FROM hostelhive.student_presence WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76');
DELETE FROM hostelhive.attendance_events WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76');
DELETE FROM hostelhive.late_access_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76');
DELETE FROM hostelhive.scanner_devices WHERE name='UPGRADE-76-device';
DELETE FROM hostelhive.announcements WHERE published_by IN(SELECT user_id FROM hostelhive.users WHERE firebase_uid IN('upgrade-76-admin','upgrade-76-warden'));
DELETE FROM hostelhive.complaint_updates WHERE complaint_id IN(SELECT complaint_id FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76'));
DELETE FROM hostelhive.complaints WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76');
DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE-76');
DELETE FROM hostelhive.students WHERE index_no='UPGRADE-76';
DELETE FROM hostelhive.users WHERE firebase_uid IN('upgrade-76-student','upgrade-76-admin','upgrade-76-warden');
DROP TABLE public.attendance_permission_upgrade_evidence;
COMMIT;