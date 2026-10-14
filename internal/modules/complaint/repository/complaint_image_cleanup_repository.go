package repository

import (
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	"github.com/jackc/pgx/v5"
)

func (s *Store) RemoveImage(ctx context.Context, uid, id, image string) (d.ImageRemoval, error) {
	tx, a, c, err := s.imageBegin(ctx, uid, id, true)
	if err != nil {
		return d.ImageRemoval{}, err
	}
	defer rollback(tx)
	m, err := scanImage(tx.QueryRow(ctx, `SELECT `+imageColumns+` FROM hostelhive.complaint_images WHERE complaint_id=$1 AND image_id=$2 FOR UPDATE`, id, image))
	if err != nil {
		return d.ImageRemoval{}, err
	}
	if m.State == "removed" {
		return d.ImageRemoval{ImageID: image, Removed: true, Version: *m.RemovedVersion, Replayed: true}, nil
	}
	if m.State != "attached" {
		return d.ImageRemoval{}, d.ErrNotFound
	}
	if c.Status != "pending" {
		return d.ImageRemoval{}, d.ErrConflict
	}
	var version int64
	err = tx.QueryRow(ctx, `UPDATE hostelhive.complaints SET version=version+1,updated_at=GREATEST(updated_at,clock_timestamp()) WHERE complaint_id=$1 RETURNING version`, id).Scan(&version)
	if err != nil {
		return d.ImageRemoval{}, dbError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,previous_status,new_status,text,previous_version,version,image_id) SELECT complaint_id,remove_action_id,$3,'image_removed',$4,$4,'Photo removed',$5::bigint-1,$5::bigint,image_id FROM hostelhive.complaint_images WHERE complaint_id=$1 AND image_id=$2`, id, image, a.id, c.Status, version)
	if err != nil {
		return d.ImageRemoval{}, dbError(err)
	}
	_, err = tx.Exec(ctx, `UPDATE hostelhive.complaint_images SET state='removed',removed_version=$3,cleanup_at=clock_timestamp() WHERE complaint_id=$1 AND image_id=$2`, id, image, version)
	if err != nil {
		return d.ImageRemoval{}, dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return d.ImageRemoval{}, dbError(err)
	}
	return d.ImageRemoval{ImageID: image, Removed: true, Version: version}, nil
}
func (s *Store) CleanupImageOne(ctx context.Context, objects s.ImageObjects) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, d.ErrUnavailable
	}
	defer rollback(tx)
	var complaint, image, key string
	err = tx.QueryRow(ctx, `SELECT complaint_id::text,image_id::text,object_key FROM hostelhive.complaint_images WHERE state IN ('reserved','removed') AND cleanup_at<=clock_timestamp() ORDER BY cleanup_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&complaint, &image, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, d.ErrUnavailable
	}
	if objects.Delete(ctx, key) != nil {
		_, err = tx.Exec(ctx, `UPDATE hostelhive.complaint_images SET cleanup_at=clock_timestamp()+interval '30 seconds' WHERE complaint_id=$1 AND image_id=$2`, complaint, image)
		if err != nil || tx.Commit(ctx) != nil {
			return true, d.ErrUnavailable
		}
		return true, d.ErrUnavailable
	}
	// Keep tombstones and audit references; expired IDs cannot resurrect old uploads.
	_, err = tx.Exec(ctx, `UPDATE hostelhive.complaint_images SET state=CASE WHEN state='reserved' THEN 'expired' ELSE state END,cleanup_at=NULL WHERE complaint_id=$1 AND image_id=$2`, complaint, image)
	if err != nil || tx.Commit(ctx) != nil {
		return true, d.ErrUnavailable
	}
	return true, nil
}
