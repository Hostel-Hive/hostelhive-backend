BEGIN;
-- Do not discard unfinished security work during rollback.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.user_revocations WHERE completed_at IS NULL) THEN
  RAISE EXCEPTION 'Pending revocations must finish before rolling back migration 4';
 END IF;
END $$;
DROP TABLE hostelhive.user_revocations RESTRICT;
COMMIT;
