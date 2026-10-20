UPDATE hostelhive.late_access_requests SET consumed_at=NULL,consumed_device_id=NULL,consumed_event_id=NULL WHERE student_id='10100000-0000-4000-8000-000000000003';
DELETE FROM hostelhive.student_presence WHERE student_id='10100000-0000-4000-8000-000000000003';
DELETE FROM hostelhive.attendance_events WHERE student_id='10100000-0000-4000-8000-000000000003';
DELETE FROM hostelhive.scanner_devices WHERE device_id='10100000-0000-4000-8000-000000000004';
DELETE FROM hostelhive.leave_requests WHERE student_id='10100000-0000-4000-8000-000000000003';
DELETE FROM hostelhive.late_access_requests WHERE student_id='10100000-0000-4000-8000-000000000003';
DELETE FROM hostelhive.students WHERE student_id='10100000-0000-4000-8000-000000000003';
DELETE FROM hostelhive.users WHERE firebase_uid LIKE 'upgrade101-%';
DROP TABLE public.pass_cancellation_upgrade_snapshot;
