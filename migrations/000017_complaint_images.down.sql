BEGIN;
DELETE FROM hostelhive.complaint_updates WHERE action IN ('image_added','image_removed');
ALTER TABLE hostelhive.complaint_updates DROP CONSTRAINT complaint_updates_image_action;
ALTER TABLE hostelhive.complaint_updates DROP CONSTRAINT complaint_updates_image_fk;
ALTER TABLE hostelhive.complaint_updates DROP COLUMN image_id;
ALTER TABLE hostelhive.complaint_updates DROP CONSTRAINT complaint_updates_action_check;
ALTER TABLE hostelhive.complaint_updates ADD CONSTRAINT complaint_updates_action_check CHECK(action IN ('created','note','status'));
DROP TABLE hostelhive.complaint_images;
COMMIT;
