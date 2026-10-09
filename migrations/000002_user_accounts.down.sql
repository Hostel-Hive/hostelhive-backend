-- Removes local account records. Review data recovery before rollback.
BEGIN;
DROP TABLE hostelhive.users RESTRICT;
DROP FUNCTION hostelhive.users_touch_updated_at() RESTRICT;
COMMIT;
