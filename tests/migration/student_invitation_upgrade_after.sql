DO $$
DECLARE actual JSONB;
BEGIN
 SELECT jsonb_build_object(
 'users',(SELECT jsonb_agg(to_jsonb(u) ORDER BY user_id) FROM hostelhive.users u),
 'students',(SELECT jsonb_agg(to_jsonb(s) ORDER BY student_id) FROM hostelhive.students s),
 'guardians',(SELECT jsonb_agg(to_jsonb(g) ORDER BY guardian_id) FROM hostelhive.guardians g),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY device_id,event_id) FROM hostelhive.attendance_events e),
 'presence',(SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p),
 'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n)) INTO actual;
 IF actual IS DISTINCT FROM (SELECT value FROM public.student_invitation_upgrade_snapshot) THEN RAISE EXCEPTION 'Invitation upgrade changed legacy records'; END IF;
 IF EXISTS(SELECT 1 FROM hostelhive.student_invitations) THEN RAISE EXCEPTION 'Invitation upgrade backfilled historical accounts'; END IF;
 IF NOT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid='migration95-legacy' AND is_active) THEN RAISE EXCEPTION 'Legacy account was deactivated'; END IF;
END;
$$;
