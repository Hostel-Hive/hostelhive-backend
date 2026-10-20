BEGIN;
DO $$
DECLARE su UUID;lid UUID;
BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.push_devices) OR EXISTS(SELECT 1 FROM hostelhive.push_deliveries) THEN RAISE EXCEPTION 'Unexpected push history/backfill';END IF;
 IF EXISTS(SELECT 1 FROM public.push_upgrade_evidence p LEFT JOIN hostelhive.notifications n ON n.notification_id::text=p.record_id WHERE p.kind='notification' AND (n.notification_id IS NULL OR to_jsonb(n)<>p.data)) THEN RAISE EXCEPTION 'Inbox/read timestamps changed';END IF;
 IF EXISTS(SELECT 1 FROM public.push_upgrade_evidence p LEFT JOIN hostelhive.notification_outbox o ON o.event_id::text||':'||o.recipient_id::text=p.record_id WHERE p.kind='outbox' AND (o.event_id IS NULL OR to_jsonb(o)<>p.data)) THEN RAISE EXCEPTION 'Pending/delivered inbox intent changed';END IF;
 -- Latest schema adds nullable cancellation audit; original source fields must
 -- still match exactly, and legacy rows must not acquire cancellation metadata.
 IF EXISTS(SELECT 1 FROM public.push_upgrade_evidence p LEFT JOIN hostelhive.leave_requests l ON l.leave_id::text=p.record_id WHERE p.kind='leave' AND (l.leave_id IS NULL OR (to_jsonb(l)-'cancelled_at'-'cancelled_by') IS DISTINCT FROM p.data OR l.cancelled_at IS NOT NULL OR l.cancelled_by IS NOT NULL)) THEN RAISE EXCEPTION 'Source leave changed';END IF;
 SELECT user_id INTO STRICT su FROM hostelhive.users WHERE firebase_uid='upgrade89-student';
 SELECT record_id::uuid INTO STRICT lid FROM public.push_upgrade_evidence WHERE kind='leave';
 INSERT INTO hostelhive.push_devices(user_id,device_id,platform,token,token_hash) VALUES(su,gen_random_uuid(),'android',repeat('T',30),decode(repeat('ab',32),'hex'));
 INSERT INTO hostelhive.notifications(event_id,recipient_id)
 SELECT o.event_id,o.recipient_id FROM hostelhive.notification_outbox o JOIN hostelhive.notification_events e USING(event_id) WHERE e.leave_id=lid AND e.kind='leave_decided';
 IF EXISTS(SELECT 1 FROM hostelhive.push_deliveries) THEN RAISE EXCEPTION 'Newly enrolled device received a historical event';END IF;
END;
$$;
DELETE FROM hostelhive.leave_requests WHERE student_id IN(SELECT student_id FROM hostelhive.students WHERE index_no='UPGRADE89');
DELETE FROM hostelhive.students WHERE index_no='UPGRADE89';
DELETE FROM hostelhive.users WHERE firebase_uid IN('upgrade89-student','upgrade89-admin');
DROP TABLE public.push_upgrade_evidence;
COMMIT;
