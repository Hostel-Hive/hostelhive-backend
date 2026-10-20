DO $$ DECLARE current_data JSONB; BEGIN
 SELECT jsonb_build_object(
 'leaves',(SELECT jsonb_agg(to_jsonb(v)-'cancelled_at'-'cancelled_by' ORDER BY leave_id) FROM hostelhive.leave_requests v),
 'late',(SELECT jsonb_agg(to_jsonb(v)-'cancelled_at'-'cancelled_by' ORDER BY late_access_id) FROM hostelhive.late_access_requests v),
 'events',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id) FROM hostelhive.notification_events v),
 'outbox',(SELECT jsonb_agg(to_jsonb(v) ORDER BY event_id,recipient_id) FROM hostelhive.notification_outbox v),
 'inbox',(SELECT jsonb_agg(to_jsonb(v) ORDER BY notification_id) FROM hostelhive.notifications v),
  'attendance',(SELECT jsonb_agg(to_jsonb(v) ORDER BY device_id,event_id) FROM hostelhive.attendance_events v),
 'presence',(SELECT jsonb_agg(to_jsonb(v) ORDER BY student_id) FROM hostelhive.student_presence v),
 'realtime',(SELECT to_jsonb(v) FROM hostelhive.attendance_publication v)) INTO current_data;
 IF current_data IS DISTINCT FROM (SELECT value FROM public.pass_cancellation_upgrade_snapshot) THEN
  RAISE EXCEPTION 'Cancellation migration changed source history, consumption, presence or inbox';
 END IF;
 IF EXISTS(SELECT 1 FROM hostelhive.leave_requests WHERE cancelled_at IS NOT NULL OR cancelled_by IS NOT NULL)
 OR EXISTS(SELECT 1 FROM hostelhive.late_access_requests WHERE cancelled_at IS NOT NULL OR cancelled_by IS NOT NULL) THEN
  RAISE EXCEPTION 'Historical cancellations must not be invented';
 END IF;
END $$;
