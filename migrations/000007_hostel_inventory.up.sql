BEGIN;
CREATE TABLE hostelhive.blocks (
 block_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 name VARCHAR(120) NOT NULL CHECK(name=btrim(name) AND name ~ '[^[:space:]]'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX blocks_name_unique ON hostelhive.blocks(lower(name));
CREATE TABLE hostelhive.rooms (
 room_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 block_id UUID NOT NULL REFERENCES hostelhive.blocks(block_id) ON DELETE RESTRICT,
 room_no VARCHAR(32) NOT NULL CHECK(room_no=btrim(room_no) AND room_no ~ '[^[:space:]]'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX rooms_number_unique ON hostelhive.rooms(block_id,lower(room_no));
CREATE TABLE hostelhive.beds (
 bed_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 room_id UUID NOT NULL REFERENCES hostelhive.rooms(room_id) ON DELETE RESTRICT,
 bed_no VARCHAR(32) NOT NULL CHECK(bed_no=btrim(bed_no) AND bed_no ~ '[^[:space:]]'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX beds_number_unique ON hostelhive.beds(room_id,lower(bed_no));
-- History remains after an allocation ends. Write APIs are a subsequent ticket.
CREATE TABLE hostelhive.bed_allocations (
 allocation_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 bed_id UUID NOT NULL REFERENCES hostelhive.beds(bed_id) ON DELETE RESTRICT,
 student_id UUID NOT NULL REFERENCES hostelhive.students(student_id) ON DELETE RESTRICT,
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 ended_at TIMESTAMPTZ,
 CHECK(ended_at IS NULL OR ended_at >= started_at)
);
CREATE UNIQUE INDEX allocations_active_bed ON hostelhive.bed_allocations(bed_id) WHERE ended_at IS NULL;
CREATE UNIQUE INDEX allocations_active_student ON hostelhive.bed_allocations(student_id) WHERE ended_at IS NULL;
CREATE INDEX allocations_student_history ON hostelhive.bed_allocations(student_id,started_at,allocation_id);
COMMIT;
