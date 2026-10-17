BEGIN;
DROP TRIGGER mobile_push_intent ON hostelhive.notifications;
DROP FUNCTION hostelhive.enqueue_mobile_push();
DROP TABLE hostelhive.push_deliveries;
DROP TABLE hostelhive.push_devices;
COMMIT;
