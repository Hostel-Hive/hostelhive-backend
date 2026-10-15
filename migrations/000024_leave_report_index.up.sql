BEGIN;
-- Submission-date reports cover all statuses, including decided requests.
CREATE INDEX leave_requests_submission_report
ON hostelhive.leave_requests(created_at DESC,leave_id DESC);
COMMIT;
