package repository

import (
	"bytes"
	"context"
	"errors"
	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/domain"
	s "github.com/Hostel-Hive/hostelhive-backend/internal/modules/complaint/service"
	"github.com/jackc/pgx/v5"
)

const imageColumns = `image_id::text,complaint_id::text,object_key,payload_hash,content_type,size_bytes,width,height,state,uploaded_at,attached_version,removed_version`

func scanImage(row pgx.Row) (d.Image, error) {
	var m d.Image
	err := row.Scan(&m.ImageID, &m.ComplaintID, &m.Key, &m.Hash, &m.ContentType, &m.Size, &m.Width, &m.Height, &m.State, &m.UploadedAt, &m.AttachedVersion, &m.RemovedVersion)
	if err != nil {
		return m, dbError(err)
	}
	return m, nil
}
func (s *Store) imageBegin(ctx context.Context, uid, id string, write bool) (pgx.Tx, actor, d.Complaint, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return nil, a, d.Complaint{}, err
	}
	if write {
		if a.role != "student" {
			rollback(tx)
			return nil, a, d.Complaint{}, d.ErrForbidden
		}
		if err = eligible(ctx, tx, a); err != nil {
			rollback(tx)
			return nil, a, d.Complaint{}, err
		}
	}
	c, err := get(ctx, tx, a, id, write)
	if err != nil {
		rollback(tx)
		return nil, a, c, err
	}
	return tx, a, c, nil
}
func (s *Store) CheckImageUpload(ctx context.Context, uid, id string) error {
	tx, _, c, err := s.imageBegin(ctx, uid, id, true)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if c.Status != "pending" {
		return d.ErrConflict
	}
	return nil
}
func (s *Store) ReserveImage(ctx context.Context, uid, id string, m d.Image) error {
	tx, a, c, err := s.imageBegin(ctx, uid, id, true)
	if err != nil {
		return err
	}
	defer rollback(tx)
	old, err := scanImage(tx.QueryRow(ctx, `SELECT `+imageColumns+` FROM hostelhive.complaint_images WHERE complaint_id=$1 AND image_id=$2 FOR UPDATE`, id, m.ImageID))
	if err == nil {
		if !bytes.Equal(old.Hash, m.Hash) || old.ContentType != m.ContentType || old.State == "removed" || old.State == "expired" {
			return d.ErrConflict
		}
		if old.State != "attached" && c.Status != "pending" {
			return d.ErrConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return dbError(err)
		}
		return nil
	}
	if !errors.Is(err, d.ErrNotFound) {
		return err
	}
	if c.Status != "pending" {
		return d.ErrConflict
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM hostelhive.complaint_images WHERE complaint_id=$1 AND state IN ('reserved','attached')`, id).Scan(&count); err != nil {
		return dbError(err)
	}
	if count >= d.MaxComplaintImages {
		return d.ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO hostelhive.complaint_images(complaint_id,image_id,object_key,payload_hash,content_type,size_bytes,width,height,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, m.ImageID, m.Key, m.Hash, m.ContentType, m.Size, m.Width, m.Height, a.id)
	if err != nil {
		return dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return dbError(err)
	}
	return nil
}

type imageWork struct {
	tx        pgx.Tx
	meta      d.Image
	actor     string
	complaint d.Complaint
}

func (w *imageWork) Close()            { rollback(w.tx) }
func (w *imageWork) Metadata() d.Image { return w.meta }
func (s *Store) BeginImage(ctx context.Context, uid, id, image string, hash []byte) (s.ImageWork, error) {
	tx, a, c, err := s.imageBegin(ctx, uid, id, true)
	if err != nil {
		return nil, err
	}
	m, err := scanImage(tx.QueryRow(ctx, `SELECT `+imageColumns+` FROM hostelhive.complaint_images WHERE complaint_id=$1 AND image_id=$2 FOR UPDATE`, id, image))
	if err != nil {
		rollback(tx)
		return nil, err
	}
	if !bytes.Equal(m.Hash, hash) || (m.State != "reserved" && m.State != "attached") || (m.State == "reserved" && c.Status != "pending") {
		rollback(tx)
		return nil, d.ErrConflict
	}
	return &imageWork{tx: tx, meta: m, actor: a.id, complaint: c}, nil
}
func (w *imageWork) Attach(ctx context.Context) (d.ImageResult, error) {
	var version int64
	err := w.tx.QueryRow(ctx, `UPDATE hostelhive.complaints SET version=version+1,updated_at=GREATEST(updated_at,clock_timestamp()) WHERE complaint_id=$1 RETURNING version`, w.meta.ComplaintID).Scan(&version)
	if err != nil {
		return d.ImageResult{}, dbError(err)
	}
	_, err = w.tx.Exec(ctx, `INSERT INTO hostelhive.complaint_updates(complaint_id,request_id,actor_id,action,previous_status,new_status,text,previous_version,version,image_id) SELECT complaint_id,upload_action_id,$3,'image_added',$4,$4,'Photo attached',$5::bigint-1,$5::bigint,image_id FROM hostelhive.complaint_images WHERE complaint_id=$1 AND image_id=$2`, w.meta.ComplaintID, w.meta.ImageID, w.actor, w.complaint.Status, version)
	if err != nil {
		return d.ImageResult{}, dbError(err)
	}
	m, err := scanImage(w.tx.QueryRow(ctx, `UPDATE hostelhive.complaint_images SET state='attached',uploaded_at=clock_timestamp(),attached_version=$3,cleanup_at=NULL WHERE complaint_id=$1 AND image_id=$2 RETURNING `+imageColumns, w.meta.ComplaintID, w.meta.ImageID, version))
	if err != nil {
		return d.ImageResult{}, err
	}
	if err = w.tx.Commit(ctx); err != nil {
		return d.ImageResult{}, dbError(err)
	}
	return d.ImageResult{Image: m, Version: version}, nil
}
func (s *Store) BeginImageRead(ctx context.Context, uid, id, image string) (s.ImageRead, error) {
	tx, _, c, err := s.imageBegin(ctx, uid, id, false)
	if err != nil {
		return nil, err
	}
	m, err := scanImage(tx.QueryRow(ctx, `SELECT `+imageColumns+` FROM hostelhive.complaint_images WHERE complaint_id=$1 AND image_id=$2 AND state='attached' FOR SHARE`, id, image))
	if err != nil {
		rollback(tx)
		return nil, err
	}
	return &imageWork{tx: tx, meta: m, complaint: c}, nil
}
func (s *Store) ListImages(ctx context.Context, uid, id string) (d.ImageList, error) {
	tx, a, err := s.begin(ctx, uid)
	if err != nil {
		return d.ImageList{}, err
	}
	defer rollback(tx)
	// Keep the returned complaint version and image list in one stable state.
	c, err := get(ctx, tx, a, id, true)
	if err != nil {
		return d.ImageList{}, err
	}
	p := d.ImageList{Items: []d.Image{}, Version: c.Version}
	rows, err := tx.Query(ctx, `SELECT `+imageColumns+` FROM hostelhive.complaint_images WHERE complaint_id=$1 AND state='attached' ORDER BY uploaded_at,image_id`, id)
	if err != nil {
		return p, dbError(err)
	}
	for rows.Next() {
		m, e := scanImage(rows)
		if e != nil {
			rows.Close()
			return p, e
		}
		p.Items = append(p.Items, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, dbError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return p, dbError(err)
	}
	return p, nil
}
