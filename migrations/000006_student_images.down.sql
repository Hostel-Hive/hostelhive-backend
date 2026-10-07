BEGIN;
ALTER TABLE hostelhive.students DROP CONSTRAINT student_image_owner;
ALTER TABLE hostelhive.students DROP COLUMN profile_image_key;
DROP TABLE hostelhive.student_image_objects;
COMMIT;
