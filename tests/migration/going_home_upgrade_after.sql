DO $$ DECLARE current_data JSONB; BEGIN
 SELECT jsonb_build_object(
 'users',(SELECT jsonb_agg(to_jsonb(u) ORDER BY user_id) FROM hostelhive.users u),
 'students',(SELECT jsonb_agg(to_jsonb(s) ORDER BY student_id) FROM hostelhive.students s),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY device_id,event_id) FROM hostelhive.attendance_events e),
 'presence',(SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p),
 'publication',(SELECT to_jsonb(p) FROM hostelhive.attendance_publication p),
 'invitations',(SELECT jsonb_agg(to_jsonb(i) ORDER BY invitation_id) FROM hostelhive.student_invitations i),
 'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n)) INTO current_data;
 IF current_data IS DISTINCT FROM (SELECT value FROM public.going_home_upgrade_snapshot) THEN
  RAISE EXCEPTION 'Going Home migration changed source records or presence/publication';
 END IF;
END $$;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.going_home_declarations) THEN RAISE EXCEPTION 'Historical attendance was backfilled as Going Home'; END IF;
END $$;
INSERT INTO hostelhive.going_home_declarations(request_id,student_id,submitted_by)
 VALUES('10000000-0000-4000-8000-000000000006','10000000-0000-4000-8000-000000000003','10000000-0000-4000-8000-000000000002');
DO $$ DECLARE current_data JSONB; BEGIN
 SELECT jsonb_build_object(
 'users',(SELECT jsonb_agg(to_jsonb(u) ORDER BY user_id) FROM hostelhive.users u),
 'students',(SELECT jsonb_agg(to_jsonb(s) ORDER BY student_id) FROM hostelhive.students s),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY device_id,event_id) FROM hostelhive.attendance_events e),
 'presence',(SELECT jsonb_agg(to_jsonb(p) ORDER BY student_id) FROM hostelhive.student_presence p),
 'publication',(SELECT to_jsonb(p) FROM hostelhive.attendance_publication p),
 'invitations',(SELECT jsonb_agg(to_jsonb(i) ORDER BY invitation_id) FROM hostelhive.student_invitations i),
 'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY notification_id) FROM hostelhive.notifications n)) INTO current_data;
 IF current_data IS DISTINCT FROM (SELECT value FROM public.going_home_upgrade_snapshot) THEN
  RAISE EXCEPTION 'Going Home migration changed source records or presence/publication';
 END IF;
END $$;
