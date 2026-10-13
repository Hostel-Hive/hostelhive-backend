BEGIN;
CREATE TABLE hostelhive.complaint_images (
 complaint_id UUID NOT NULL REFERENCES hostelhive.complaints(complaint_id) ON DELETE RESTRICT,
 image_id UUID NOT NULL,
 object_key TEXT NOT NULL UNIQUE,
 payload_hash BYTEA NOT NULL CHECK(octet_length(payload_hash)=32),
 content_type VARCHAR(16) NOT NULL CHECK(content_type IN ('image/jpeg','image/png')),
 size_bytes INTEGER NOT NULL CHECK(size_bytes BETWEEN 1 AND 5242880),
 width INTEGER NOT NULL CHECK(width BETWEEN 1 AND 4096),
 height INTEGER NOT NULL CHECK(height BETWEEN 1 AND 4096 AND width::bigint*height<=12000000),
 state VARCHAR(16) NOT NULL DEFAULT 'reserved' CHECK(state IN ('reserved','attached','removed','expired')),
 uploaded_by UUID NOT NULL REFERENCES hostelhive.users(user_id) ON DELETE RESTRICT,
 uploaded_at TIMESTAMPTZ,
 attached_version BIGINT,
 removed_version BIGINT,
 upload_action_id UUID NOT NULL DEFAULT gen_random_uuid(),
 remove_action_id UUID NOT NULL DEFAULT gen_random_uuid(),
 cleanup_at TIMESTAMPTZ DEFAULT clock_timestamp()+interval '15 minutes',
 PRIMARY KEY(complaint_id,image_id),
 CHECK((state='attached' AND uploaded_at IS NOT NULL AND attached_version IS NOT NULL AND cleanup_at IS NULL) OR state<>'attached')
);
CREATE INDEX complaint_images_cleanup ON hostelhive.complaint_images(cleanup_at) WHERE cleanup_at IS NOT NULL;
ALTER TABLE hostelhive.complaint_updates DROP CONSTRAINT complaint_updates_action_check;
ALTER TABLE hostelhive.complaint_updates ADD CONSTRAINT complaint_updates_action_check CHECK(action IN ('created','note','status','image_added','image_removed'));
ALTER TABLE hostelhive.complaint_updates ADD COLUMN image_id UUID;
ALTER TABLE hostelhive.complaint_updates ADD CONSTRAINT complaint_updates_image_fk FOREIGN KEY(complaint_id,image_id) REFERENCES hostelhive.complaint_images(complaint_id,image_id) ON DELETE RESTRICT;
ALTER TABLE hostelhive.complaint_updates ADD CONSTRAINT complaint_updates_image_action CHECK((action IN ('image_added','image_removed'))=(image_id IS NOT NULL));
COMMIT;
