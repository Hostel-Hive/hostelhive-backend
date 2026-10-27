BEGIN;
-- Never silently reopen or delete cancelled requests during rollback.
LOCK TABLE hostelhive.leave_requests, hostelhive.late_access_requests IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.leave_requests WHERE status='cancelled')
 OR EXISTS(SELECT 1 FROM hostelhive.late_access_requests WHERE status='cancelled') THEN
  RAISE EXCEPTION 'Cannot roll back pass cancellation while cancelled requests exist; retain migration 28 to preserve history';
 END IF;
END $$;
ALTER TABLE hostelhive.leave_requests DROP CONSTRAINT leave_requests_cancellation_check;
ALTER TABLE hostelhive.leave_requests DROP COLUMN cancelled_at, DROP COLUMN cancelled_by;
ALTER TABLE hostelhive.leave_requests DROP CONSTRAINT leave_requests_status_check;
ALTER TABLE hostelhive.leave_requests DROP CONSTRAINT leave_requests_check1;
DROP TRIGGER leave_notifications ON hostelhive.leave_requests;
ALTER TABLE hostelhive.leave_requests ALTER COLUMN status TYPE VARCHAR(8);
CREATE TRIGGER leave_notifications AFTER INSERT OR UPDATE OF status ON hostelhive.leave_requests
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_leave_notifications();
ALTER TABLE hostelhive.leave_requests ADD CONSTRAINT leave_requests_status_check CHECK(status IN ('pending','approved','rejected'));
ALTER TABLE hostelhive.leave_requests ADD CONSTRAINT leave_requests_check1 CHECK(
 (status='pending' AND decided_at IS NULL AND decided_by IS NULL AND decision_note='')
 OR (status IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL));
ALTER TABLE hostelhive.late_access_requests DROP CONSTRAINT late_access_requests_cancellation_check;
ALTER TABLE hostelhive.late_access_requests DROP COLUMN cancelled_at, DROP COLUMN cancelled_by;
ALTER TABLE hostelhive.late_access_requests DROP CONSTRAINT late_access_requests_status_check;
ALTER TABLE hostelhive.late_access_requests DROP CONSTRAINT late_access_requests_check;
DROP TRIGGER late_access_notifications ON hostelhive.late_access_requests;
ALTER TABLE hostelhive.late_access_requests ALTER COLUMN status TYPE VARCHAR(8);
CREATE TRIGGER late_access_notifications AFTER INSERT OR UPDATE OF status ON hostelhive.late_access_requests
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_late_access_notifications();
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_requests_status_check CHECK(status IN ('pending','approved','rejected'));
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_requests_check CHECK(
 (status='pending' AND decided_at IS NULL AND decided_by IS NULL AND decision_note='')
 OR (status IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL));
COMMIT;
