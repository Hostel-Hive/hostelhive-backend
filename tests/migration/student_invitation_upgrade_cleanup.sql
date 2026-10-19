-- Remove only this disposable migration test's records before integration tests.
BEGIN;
DELETE FROM hostelhive.guardians WHERE student_id='95000000-0000-4000-8000-000000000011';
DELETE FROM hostelhive.students WHERE student_id IN('95000000-0000-4000-8000-000000000011','95000000-0000-4000-8000-000000000013');
DELETE FROM hostelhive.user_revocations WHERE user_id='95000000-0000-4000-8000-000000000003';
DELETE FROM hostelhive.users WHERE user_id IN('95000000-0000-4000-8000-000000000001','95000000-0000-4000-8000-000000000002','95000000-0000-4000-8000-000000000003');
DROP TABLE public.student_invitation_upgrade_snapshot;
COMMIT;
