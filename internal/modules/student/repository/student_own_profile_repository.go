package repository

import (
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/jackc/pgx/v5"
)

// OwnProfile reads one consistent snapshot without modifying source tables.
func (s *Store) OwnProfile(ctx context.Context, actor string) (d.OwnProfile, error) {
	var result d.OwnProfile
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, d.ErrUnavailable
	}
	defer rollback(tx)
	var allowed bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role='student')", actor).Scan(&allowed); err != nil {
		return result, d.ErrUnavailable
	}
	if !allowed {
		return result, d.ErrForbidden
	}
	var id string
	if err = tx.QueryRow(ctx, "SELECT s.student_id::text FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE u.firebase_uid=$1 AND s.deleted_at IS NULL", actor).Scan(&id); err != nil {
		return result, dbError(err)
	}
	result, err = ownProfile(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if tx.Commit(ctx) != nil {
		return d.OwnProfile{}, d.ErrUnavailable
	}
	return result, nil
}

// Called inside either the read snapshot or the owning Student's update transaction.
func ownProfile(ctx context.Context, tx pgx.Tx, id string) (d.OwnProfile, error) {
	var result d.OwnProfile
	p, err := get(ctx, tx, id)
	if err != nil {
		return result, err
	}
	result = d.OwnProfile{StudentID: p.StudentID, IndexNo: p.IndexNo, FullName: p.FullName, Faculty: p.Faculty, Year: p.Year, ContactPhone: p.ContactPhone, Email: p.Email, Guardians: p.Guardians}
	if p.ProfileImageURL != "" {
		url := "/api/v1/me/student-profile/image"
		result.ProfileImageURL = &url
	}
	var a d.OwnAccommodation
	err = tx.QueryRow(ctx, `SELECT a.allocation_id::text,b.block_id::text,b.name,r.room_id::text,r.room_no,bed.bed_id::text,bed.bed_no,a.started_at FROM hostelhive.bed_allocations a JOIN hostelhive.beds bed USING(bed_id) JOIN hostelhive.rooms r USING(room_id) JOIN hostelhive.blocks b USING(block_id) WHERE a.student_id=$1 AND a.ended_at IS NULL`, id).Scan(&a.AllocationID, &a.BlockID, &a.BlockName, &a.RoomID, &a.RoomNo, &a.BedID, &a.BedNo, &a.StartedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, d.ErrUnavailable
	}
	if err == nil {
		result.Accommodation = &a
	}
	result.Presence.Status = "UNKNOWN"
	err = tx.QueryRow(ctx, "SELECT status,updated_at FROM hostelhive.student_presence WHERE student_id=$1", id).Scan(&result.Presence.Status, &result.Presence.UpdatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, d.ErrUnavailable
	}
	return result, nil
}
