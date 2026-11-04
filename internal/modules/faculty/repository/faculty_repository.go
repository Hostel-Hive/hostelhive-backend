package repository

import (
	"context"
	"errors"
	"time"

	d "github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/faculty/dto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store { return &Store{p} }

const columns = "faculty_id::text,name,is_active,created_at,updated_at"

func failure(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return d.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(e, &p) && p.Code == "23505" {
		return d.ErrConflict
	}
	return d.ErrUnavailable
}
func scan(row pgx.Row) (d.Faculty, error) {
	var v d.Faculty
	e := row.Scan(&v.FacultyID, &v.Name, &v.IsActive, &v.CreatedAt, &v.UpdatedAt)
	if e != nil {
		return v, failure(e)
	}
	return v, nil
}
func rollback(tx pgx.Tx) {
	ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
	defer c()
	_ = tx.Rollback(ctx)
}
func (s *Store) begin(ctx context.Context, actor string, write bool) (pgx.Tx, string, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, "", d.ErrUnavailable
	}
	lock := "LOCK TABLE hostelhive.users IN SHARE MODE"
	if write {
		lock = "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"
	}
	if _, e = tx.Exec(ctx, lock); e != nil {
		rollback(tx)
		return nil, "", d.ErrUnavailable
	}
	var id, role string
	e = tx.QueryRow(ctx, `SELECT user_id::text,role FROM hostelhive.users WHERE firebase_uid=$1 AND is_active AND role IN ('admin','warden') AND NOT EXISTS(SELECT 1 FROM hostelhive.student_invitations i WHERE i.user_id=users.user_id AND i.setup_completed_at IS NULL)`, actor).Scan(&id, &role)
	if e != nil {
		rollback(tx)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, "", d.ErrForbidden
		}
		return nil, "", d.ErrUnavailable
	}
	if write && role != "admin" {
		rollback(tx)
		return nil, "", d.ErrForbidden
	}
	return tx, id, nil
}
func (s *Store) List(ctx context.Context, actor string, f d.Filter) (d.Page, error) {
	page := d.Page{Items: []d.Faculty{}, Limit: f.Limit, Offset: f.Offset}
	tx, _, e := s.begin(ctx, actor, false)
	if e != nil {
		return page, e
	}
	defer rollback(tx)
	rows, e := tx.Query(ctx, "SELECT "+columns+" FROM hostelhive.faculties WHERE ($1::boolean IS NULL OR is_active=$1) ORDER BY lower(name),faculty_id LIMIT $2 OFFSET $3", f.Active, f.Limit+1, f.Offset)
	if e != nil {
		return page, d.ErrUnavailable
	}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			rows.Close()
			return page, e
		}
		page.Items = append(page.Items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return page, d.ErrUnavailable
	}
	if len(page.Items) > f.Limit {
		page.Items = page.Items[:f.Limit]
		page.HasMore = true
	}
	if tx.Commit(ctx) != nil {
		return page, d.ErrUnavailable
	}
	return page, nil
}
func (s *Store) Get(ctx context.Context, actor, id string) (d.Faculty, error) {
	tx, _, e := s.begin(ctx, actor, false)
	if e != nil {
		return d.Faculty{}, e
	}
	defer rollback(tx)
	v, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.faculties WHERE faculty_id=$1", id))
	if e != nil {
		return v, e
	}
	if tx.Commit(ctx) != nil {
		return v, d.ErrUnavailable
	}
	return v, nil
}
func audit(ctx context.Context, tx pgx.Tx, id, actor, action string, old *string, name string) error {
	_, e := tx.Exec(ctx, `INSERT INTO hostelhive.faculty_audit(faculty_id,actor_id,action,old_name,new_name) VALUES($1,$2,$3,$4,$5)`, id, actor, action, old, name)
	if e != nil {
		return d.ErrUnavailable
	}
	return nil
}
func (s *Store) Create(ctx context.Context, actor string, in dto.Details) (d.Faculty, error) {
	tx, uid, e := s.begin(ctx, actor, true)
	if e != nil {
		return d.Faculty{}, e
	}
	defer rollback(tx)
	v, e := scan(tx.QueryRow(ctx, "INSERT INTO hostelhive.faculties(name,created_by,updated_by) VALUES($1,$2,$2) RETURNING "+columns, in.Name, uid))
	if e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO hostelhive.faculty_names(name_key,faculty_id) VALUES(lower($1),$2)`, v.Name, v.FacultyID); e != nil {
		return v, failure(e)
	}
	if e = audit(ctx, tx, v.FacultyID, uid, "created", nil, v.Name); e != nil {
		return v, e
	}
	if tx.Commit(ctx) != nil {
		return v, d.ErrUnavailable
	}
	return v, nil
}
func (s *Store) Rename(ctx context.Context, actor, id string, in dto.Details) (d.Faculty, error) {
	tx, uid, e := s.begin(ctx, actor, true)
	if e != nil {
		return d.Faculty{}, e
	}
	defer rollback(tx)
	old, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.faculties WHERE faculty_id=$1 FOR UPDATE", id))
	if e != nil {
		return old, e
	}
	if old.Name == in.Name {
		if tx.Commit(ctx) != nil {
			return old, d.ErrUnavailable
		}
		return old, nil
	}
	// Reserve every former name permanently to this identity, including inactive faculties.
	var reserved string
	e = tx.QueryRow(ctx, "SELECT faculty_id::text FROM hostelhive.faculty_names WHERE name_key=lower($1)", in.Name).Scan(&reserved)
	if e == nil && reserved != id {
		return d.Faculty{}, d.ErrConflict
	}
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return d.Faculty{}, d.ErrUnavailable
	}
	if _, e = tx.Exec(ctx, `INSERT INTO hostelhive.faculty_names(name_key,faculty_id) VALUES(lower($1),$2) ON CONFLICT DO NOTHING`, in.Name, id); e != nil {
		return d.Faculty{}, failure(e)
	}
	v, e := scan(tx.QueryRow(ctx, "UPDATE hostelhive.faculties SET name=$2,updated_at=clock_timestamp(),updated_by=$3 WHERE faculty_id=$1 RETURNING "+columns, id, in.Name, uid))
	if e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, `UPDATE hostelhive.students SET faculty=$2,updated_at=clock_timestamp() WHERE lower(btrim(faculty)) IN(SELECT name_key FROM hostelhive.faculty_names WHERE faculty_id=$1)`, id, in.Name); e != nil {
		return v, d.ErrUnavailable
	}
	if e = audit(ctx, tx, id, uid, "renamed", &old.Name, v.Name); e != nil {
		return v, e
	}
	if tx.Commit(ctx) != nil {
		return v, d.ErrUnavailable
	}
	return v, nil
}
func (s *Store) SetActive(ctx context.Context, actor, id string, active bool) (d.Faculty, error) {
	tx, uid, e := s.begin(ctx, actor, true)
	if e != nil {
		return d.Faculty{}, e
	}
	defer rollback(tx)
	v, e := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hostelhive.faculties WHERE faculty_id=$1 FOR UPDATE", id))
	if e != nil {
		return v, e
	}
	if v.IsActive != active {
		v, e = scan(tx.QueryRow(ctx, "UPDATE hostelhive.faculties SET is_active=$2,updated_at=clock_timestamp(),updated_by=$3 WHERE faculty_id=$1 RETURNING "+columns, id, active, uid))
		if e != nil {
			return v, e
		}
		action := "deactivated"
		if active {
			action = "activated"
		}
		if e = audit(ctx, tx, id, uid, action, nil, v.Name); e != nil {
			return v, e
		}
	}
	if tx.Commit(ctx) != nil {
		return v, d.ErrUnavailable
	}
	return v, nil
}
