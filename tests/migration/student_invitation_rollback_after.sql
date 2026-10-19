DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid='migration95-pending' AND is_active) THEN RAISE EXCEPTION 'Rollback granted access to a pending Student'; END IF;
 IF NOT EXISTS(SELECT 1 FROM hostelhive.user_revocations WHERE user_id='95000000-0000-4000-8000-000000000003' AND completed_at IS NULL) THEN RAISE EXCEPTION 'Rollback omitted pending revocation'; END IF;
 IF NOT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid='migration95-legacy' AND is_active) THEN RAISE EXCEPTION 'Rollback changed legacy access'; END IF;
END $$;
