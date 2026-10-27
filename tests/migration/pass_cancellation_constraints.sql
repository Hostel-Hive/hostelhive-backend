-- Both valid cancellations and invalid metadata are checked inside a rolled-back fixture.
BEGIN;
DO $$ DECLARE tab TEXT; key TEXT; id UUID; owner_id UUID:='10100000-0000-4000-8000-000000000002'; BEGIN
 FOREACH tab IN ARRAY ARRAY['leave_requests','late_access_requests'] LOOP
  key:=CASE WHEN tab='leave_requests' THEN 'leave_id' ELSE 'late_access_id' END;
  EXECUTE format('SELECT %I FROM hostelhive.%I WHERE status=''pending'' AND submitted_by=$1 LIMIT 1',key,tab) INTO STRICT id USING owner_id;
  BEGIN
   EXECUTE format('UPDATE hostelhive.%I SET status=''cancelled'' WHERE %I=$1',tab,key) USING id;
   RAISE EXCEPTION 'Cancellation without audit was accepted';
  EXCEPTION WHEN check_violation THEN NULL; END;
  BEGIN
   EXECUTE format('UPDATE hostelhive.%I SET cancelled_at=clock_timestamp(),cancelled_by=$2 WHERE %I=$1',tab,key) USING id,owner_id;
   RAISE EXCEPTION 'Pending request accepted cancellation audit';
  EXCEPTION WHEN check_violation THEN NULL; END;
  BEGIN
   EXECUTE format('UPDATE hostelhive.%I SET status=''cancelled'',cancelled_at=clock_timestamp(),cancelled_by=''10100000-0000-4000-8000-000000000001'' WHERE %I=$1',tab,key) USING id;
   RAISE EXCEPTION 'Cancellation by a non-owner was accepted';
  EXCEPTION WHEN check_violation THEN NULL; END;
  BEGIN
   EXECUTE format('UPDATE hostelhive.%I SET status=''cancelled'',cancelled_at=''infinity'',cancelled_by=$2 WHERE %I=$1',tab,key) USING id,owner_id;
   RAISE EXCEPTION 'Infinite cancellation timestamp accepted';
  EXCEPTION WHEN check_violation THEN NULL; END;
  EXECUTE format('UPDATE hostelhive.%I SET status=''cancelled'',cancelled_at=clock_timestamp(),cancelled_by=$2 WHERE %I=$1',tab,key) USING id,owner_id;
  BEGIN
   EXECUTE format('UPDATE hostelhive.%I SET decided_at=clock_timestamp(),decided_by=$2 WHERE %I=$1',tab,key) USING id,owner_id;
   RAISE EXCEPTION 'Cancellation accepted a staff decision';
  EXCEPTION WHEN check_violation THEN NULL; END;
 END LOOP;
END $$;
ROLLBACK;
