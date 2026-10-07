-- RESTRICT (the default) refuses rollback if later objects still depend on this schema.
BEGIN;
DROP SCHEMA hostelhive RESTRICT;
COMMIT;
