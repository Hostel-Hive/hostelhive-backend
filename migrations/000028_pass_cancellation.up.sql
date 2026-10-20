BEGIN;
-- Add a terminal cancellation state without changing existing decisions or alerts.
ALTER TABLE hostelhive.leave_requests DROP CONSTRAINT leave_requests_status_check;
ALTER TABLE hostelhive.leave_requests DROP CONSTRAINT leave_requests_check1;
DROP TRIGGER leave_notifications ON hostelhive.leave_requests;
ALTER TABLE hostelhive.leave_requests ALTER COLUMN status TYPE VARCHAR(9);
CREATE TRIGGER leave_notifications AFTER INSERT OR UPDATE OF status ON hostelhive.leave_requests
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_leave_notifications();
ALTER TABLE hostelhive.leave_requests ADD COLUMN cancelled_at TIMESTAMPTZ;
ALTER TABLE hostelhive.leave_requests ADD COLUMN cancelled_by UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT;
ALTER TABLE hostelhive.leave_requests ADD CONSTRAINT leave_requests_status_check CHECK(status IN ('pending','approved','rejected','cancelled'));
ALTER TABLE hostelhive.leave_requests ADD CONSTRAINT leave_requests_check1 CHECK(
 (status IN ('pending','cancelled') AND decided_at IS NULL AND decided_by IS NULL AND decision_note='')
 OR (status IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL));
ALTER TABLE hostelhive.leave_requests ADD CONSTRAINT leave_requests_cancellation_check CHECK(
 (status='cancelled' AND cancelled_at IS NOT NULL AND isfinite(cancelled_at) AND cancelled_at>=created_at AND cancelled_by IS NOT NULL AND cancelled_by=submitted_by)
 OR (status<>'cancelled' AND cancelled_at IS NULL AND cancelled_by IS NULL));
ALTER TABLE hostelhive.late_access_requests DROP CONSTRAINT late_access_requests_status_check;
ALTER TABLE hostelhive.late_access_requests DROP CONSTRAINT late_access_requests_check;
DROP TRIGGER late_access_notifications ON hostelhive.late_access_requests;
ALTER TABLE hostelhive.late_access_requests ALTER COLUMN status TYPE VARCHAR(9);
CREATE TRIGGER late_access_notifications AFTER INSERT OR UPDATE OF status ON hostelhive.late_access_requests
 FOR EACH ROW EXECUTE FUNCTION hostelhive.enqueue_late_access_notifications();
ALTER TABLE hostelhive.late_access_requests ADD COLUMN cancelled_at TIMESTAMPTZ;
ALTER TABLE hostelhive.late_access_requests ADD COLUMN cancelled_by UUID REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT;
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_requests_status_check CHECK(status IN ('pending','approved','rejected','cancelled'));
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_requests_check CHECK(
 (status IN ('pending','cancelled') AND decided_at IS NULL AND decided_by IS NULL AND decision_note='')
 OR (status IN ('approved','rejected') AND decided_at IS NOT NULL AND decided_by IS NOT NULL));
ALTER TABLE hostelhive.late_access_requests ADD CONSTRAINT late_access_requests_cancellation_check CHECK(
 (status='cancelled' AND cancelled_at IS NOT NULL AND isfinite(cancelled_at) AND cancelled_at>=created_at AND cancelled_by IS NOT NULL AND cancelled_by=submitted_by)
 OR (status<>'cancelled' AND cancelled_at IS NULL AND cancelled_by IS NULL));
COMMIT;
