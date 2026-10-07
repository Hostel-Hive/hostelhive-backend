-- Removes retry metadata only; does not delete local or Firebase accounts.
BEGIN;
DROP TABLE hostelhive.user_provisioning RESTRICT;
COMMIT;
