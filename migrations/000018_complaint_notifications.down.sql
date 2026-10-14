BEGIN;
DROP TRIGGER complaint_notifications ON hostelhive.complaint_updates;
DROP FUNCTION hostelhive.enqueue_complaint_notifications();
DROP TABLE hostelhive.notifications;
DROP TABLE hostelhive.notification_outbox;
COMMIT;
