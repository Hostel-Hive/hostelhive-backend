BEGIN;
-- Rollback must not silently remove setup restrictions from uncompleted accounts.
UPDATE hostelhive.users SET is_active=false WHERE user_id IN
 (SELECT user_id FROM hostelhive.student_invitations WHERE setup_completed_at IS NULL);
INSERT INTO hostelhive.user_revocations(user_id)
 SELECT user_id FROM hostelhive.student_invitations WHERE setup_completed_at IS NULL
 ON CONFLICT(user_id) DO UPDATE SET requested_at=clock_timestamp(),next_attempt_at=clock_timestamp(),completed_at=NULL;
DROP TABLE hostelhive.student_invitation_audit;
DROP TABLE hostelhive.student_invitation_resends;
DROP TABLE hostelhive.student_invitations;
ALTER TABLE hostelhive.students DROP CONSTRAINT student_account_pair;
COMMIT;
