package repository

import (
	"context"
	"errors"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/service"
	"github.com/jackc/pgx/v5"
)

func lockStudent(ctx context.Context, tx pgx.Tx, id string) error {
	var found string
	err := tx.QueryRow(ctx, "SELECT student_id::text FROM hostelhive.students WHERE student_id=$1 AND deleted_at IS NULL FOR UPDATE", id).Scan(&found)
	if err != nil {
		return dbError(err)
	}
	return nil
}
func (r *Store) ReserveImage(ctx context.Context, actor, id string, m d.Image) error {
	tx, err := r.begin(ctx, actor)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = lockStudent(ctx, tx, id); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO hostelhive.student_image_objects(object_key,student_id,content_type,size_bytes,width,height) VALUES($1,$2,$3,$4,$5,$6)`, m.Key, id, m.ContentType, m.Size, m.Width, m.Height)
	if err != nil || tx.Commit(ctx) != nil {
		return d.ErrUnavailable
	}
	return nil
}

type imageWork struct {
	tx      pgx.Tx
	id, key string
}

func (w *imageWork) Close() { rollback(w.tx) }
func (r *Store) BeginImage(ctx context.Context, actor, id, key string) (s.ImageWork, error) {
	tx, err := r.begin(ctx, actor)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			rollback(tx)
		}
	}()
	if err = lockStudent(ctx, tx, id); err != nil {
		return nil, err
	}
	var found string
	if err = tx.QueryRow(ctx, "SELECT object_key FROM hostelhive.student_image_objects WHERE object_key=$1 AND student_id=$2 AND cleanup_at IS NOT NULL FOR UPDATE", key, id).Scan(&found); err != nil {
		return nil, d.ErrUnavailable
	}
	keep = true
	return &imageWork{tx: tx, id: id, key: key}, nil
}
func (w *imageWork) Attach(ctx context.Context) (d.Profile, error) {
	if _, err := w.tx.Exec(ctx, `UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp() WHERE object_key=(SELECT profile_image_key FROM hostelhive.students WHERE student_id=$1)`, w.id); err != nil {
		return d.Profile{}, d.ErrUnavailable
	}
	if _, err := w.tx.Exec(ctx, "UPDATE hostelhive.students SET profile_image_key=$2 WHERE student_id=$1", w.id, w.key); err != nil {
		return d.Profile{}, d.ErrUnavailable
	}
	if _, err := w.tx.Exec(ctx, "UPDATE hostelhive.student_image_objects SET cleanup_at=NULL WHERE object_key=$1", w.key); err != nil {
		return d.Profile{}, d.ErrUnavailable
	}
	profile, err := get(ctx, w.tx, w.id)
	if err != nil {
		return profile, err
	}
	if w.tx.Commit(ctx) != nil {
		return d.Profile{}, d.ErrUnavailable
	}
	return profile, nil
}
func (r *Store) GetImage(ctx context.Context, id string) (d.Image, error) {
	var m d.Image
	err := r.pool.QueryRow(ctx, `SELECT o.object_key,o.content_type,o.size_bytes,o.width,o.height FROM hostelhive.student_image_objects o JOIN hostelhive.students s ON s.profile_image_key=o.object_key WHERE s.student_id=$1 AND s.deleted_at IS NULL`, id).Scan(&m.Key, &m.ContentType, &m.Size, &m.Width, &m.Height)
	if err != nil {
		return m, dbError(err)
	}
	return m, nil
}
func (r *Store) RemoveImage(ctx context.Context, actor, id string) error {
	tx, err := r.begin(ctx, actor)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = lockStudent(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp() WHERE object_key=(SELECT profile_image_key FROM hostelhive.students WHERE student_id=$1)`, id); err != nil {
		return d.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "UPDATE hostelhive.students SET profile_image_key=NULL WHERE student_id=$1", id); err != nil || tx.Commit(ctx) != nil {
		return d.ErrUnavailable
	}
	return nil
}

// Reserved uploads survive rollback and are eligible for cleanup after 15 minutes.
// Row locks prevent deletion while upload/attachment is in progress.
func (r *Store) CleanupOne(ctx context.Context, objects s.Objects) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, d.ErrUnavailable
	}
	defer rollback(tx)
	var key string
	err = tx.QueryRow(ctx, `SELECT object_key FROM hostelhive.student_image_objects WHERE cleanup_at<=clock_timestamp() ORDER BY cleanup_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, d.ErrUnavailable
	}
	var used bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.students WHERE profile_image_key=$1)", key).Scan(&used); err != nil {
		return false, d.ErrUnavailable
	}
	if used {
		_, err = tx.Exec(ctx, "UPDATE hostelhive.student_image_objects SET cleanup_at=NULL WHERE object_key=$1", key)
	} else if objects.Delete(ctx, key) != nil {
		_, err = tx.Exec(ctx, "UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp()+interval '30 seconds' WHERE object_key=$1", key)
		if err != nil || tx.Commit(ctx) != nil {
			return false, d.ErrUnavailable
		}
		return true, d.ErrUnavailable
	} else {
		_, err = tx.Exec(ctx, "DELETE FROM hostelhive.student_image_objects WHERE object_key=$1", key)
	}
	if err != nil || tx.Commit(ctx) != nil {
		return false, d.ErrUnavailable
	}
	return true, nil
}
