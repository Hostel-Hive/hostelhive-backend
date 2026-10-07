BEGIN;
CREATE TABLE hostelhive.student_image_objects (
 object_key TEXT PRIMARY KEY CHECK(object_key ~ '^student-images/[0-9a-f-]{36}/[0-9a-f]{32}$'),
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 content_type TEXT NOT NULL CHECK(content_type IN ('image/jpeg','image/png')),
 size_bytes INTEGER NOT NULL CHECK(size_bytes BETWEEN 1 AND 5242880),
 width INTEGER NOT NULL CHECK(width BETWEEN 1 AND 4096),
 height INTEGER NOT NULL CHECK(height BETWEEN 1 AND 4096),
 CHECK(width::bigint*height::bigint<=12000000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 cleanup_at TIMESTAMPTZ DEFAULT clock_timestamp()+interval '15 minutes',
 UNIQUE(student_id,object_key)
);
ALTER TABLE hostelhive.students ADD COLUMN profile_image_key TEXT;
ALTER TABLE hostelhive.students ADD CONSTRAINT student_image_owner
 FOREIGN KEY(student_id,profile_image_key) REFERENCES hostelhive.student_image_objects(student_id,object_key);
CREATE INDEX student_images_cleanup ON hostelhive.student_image_objects(cleanup_at) WHERE cleanup_at IS NOT NULL;
COMMIT;
