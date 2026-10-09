package repository

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	sdto "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/dto"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const profileColumns = "s.student_id::text,s.user_id::text,s.index_no,s.full_name,s.faculty,s.year,s.contact_phone,u.email,u.is_active,s.created_at,s.updated_at,s.profile_image_key"

func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return sdomain.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && (p.Code == "23505") {
		return sdomain.ErrConflict
	}
	return sdomain.ErrUnavailable
}

func scan(row pgx.Row) (sdomain.Profile, error) {
	var v sdomain.Profile
	var imageKey *string
	err := row.Scan(&v.StudentID, &v.UserID, &v.IndexNo, &v.FullName, &v.Faculty, &v.Year, &v.ContactPhone, &v.Email, &v.AccountActive, &v.CreatedAt, &v.UpdatedAt, &imageKey)
	if err != nil {
		return v, dbError(err)
	}
	if imageKey != nil {
		v.ProfileImageURL = "/api/v1/students/" + v.StudentID + "/image"
	}
	return v, nil
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func get(ctx context.Context, q querier, id string) (sdomain.Profile, error) {
	v, err := scan(q.QueryRow(ctx, "SELECT "+profileColumns+" FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE s.student_id=$1 AND s.deleted_at IS NULL", id))
	if err != nil {
		return v, err
	}
	rows, err := q.Query(ctx, "SELECT guardian_id::text,name,relationship,contact_phone FROM hostelhive.guardians WHERE student_id=$1 ORDER BY guardian_id", id)
	if err != nil {
		return v, sdomain.ErrUnavailable
	}
	defer rows.Close()
	v.Guardians = []sdomain.Guardian{}
	for rows.Next() {
		var g sdomain.Guardian
		if rows.Scan(&g.GuardianID, &g.Name, &g.Relationship, &g.ContactPhone) != nil {
			return v, sdomain.ErrUnavailable
		}
		v.Guardians = append(v.Guardians, g)
	}
	if rows.Err() != nil {
		return v, sdomain.ErrUnavailable
	}
	return v, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func (s *Store) Get(ctx context.Context, id string) (sdomain.Profile, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return sdomain.Profile{}, sdomain.ErrUnavailable
	}
	defer rollback(tx)
	v, err := get(ctx, tx, id)
	if err != nil {
		return v, err
	}
	if tx.Commit(ctx) != nil {
		return sdomain.Profile{}, sdomain.ErrUnavailable
	}
	return v, nil
}

func (s *Store) List(ctx context.Context, f sdomain.Filter) (sdomain.Page, error) {
	page := sdomain.Page{Students: []sdomain.Profile{}, Limit: f.Limit, Offset: f.Offset}
	rows, err := s.pool.Query(ctx, "SELECT "+profileColumns+` FROM hostelhive.students s JOIN hostelhive.users u USING(user_id)
 WHERE s.deleted_at IS NULL AND ($1::text='' OR strpos(lower(s.index_no),lower($1))>0 OR strpos(lower(s.full_name),lower($1))>0)
 AND ($2::text='' OR lower(s.faculty)=lower($2)) AND ($3::int=0 OR s.year=$3)
 ORDER BY s.created_at,s.student_id LIMIT $4 OFFSET $5`, f.Q, f.Faculty, f.Year, f.Limit+1, f.Offset)
	if err != nil {
		return page, sdomain.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return page, err
		}
		page.Students = append(page.Students, v)
	}
	if rows.Err() != nil {
		return page, sdomain.ErrUnavailable
	}
	if len(page.Students) > f.Limit {
		page.HasMore = true
		page.Students = page.Students[:f.Limit]
	}
	return page, nil
}

func (s *Store) begin(ctx context.Context, actor string) (pgx.Tx, error) {
	return s.beginPolicy(ctx, actor, false)
}
func (s *Store) beginPolicy(ctx context.Context, actor string, adminOnly bool) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, sdomain.ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		rollback(tx)
		return nil, sdomain.ErrUnavailable
	}
	var ok bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND role IN ('admin','warden') AND is_active AND (NOT $2::boolean OR role='admin'))", actor, adminOnly).Scan(&ok)
	if err != nil {
		rollback(tx)
		return nil, sdomain.ErrUnavailable
	}
	if !ok {
		rollback(tx)
		return nil, sdomain.ErrForbidden
	}
	return tx, nil
}

func guardians(ctx context.Context, tx pgx.Tx, id string, gs []sdomain.GuardianInput) error {
	for _, g := range gs {
		if _, err := tx.Exec(ctx, "INSERT INTO hostelhive.guardians(student_id,name,relationship,contact_phone) VALUES($1,$2,$3,$4)", id, g.Name, g.Relationship, g.ContactPhone); err != nil {
			return dbError(err)
		}
	}
	return nil
}

func (s *Store) Create(ctx context.Context, actor string, input sdto.CreateInput) (sdomain.Profile, error) {
	return s.create(ctx, actor, input, false)
}

// CreateImport rechecks active Admin authority under the users write lock.
func (s *Store) CreateImport(ctx context.Context, actor string, input sdto.CreateInput) (sdomain.Profile, error) {
	return s.create(ctx, actor, input, true)
}
func (s *Store) create(ctx context.Context, actor string, input sdto.CreateInput, adminOnly bool) (sdomain.Profile, error) {
	v := input.Details
	tx, err := s.beginPolicy(ctx, actor, adminOnly)
	if err != nil {
		return sdomain.Profile{}, err
	}
	defer rollback(tx)
	var eligible bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE user_id=$1 AND role='student' AND is_active)", input.UserID).Scan(&eligible)
	if err != nil {
		return sdomain.Profile{}, sdomain.ErrUnavailable
	}
	if !eligible {
		return sdomain.Profile{}, sdomain.ErrAccount
	}
	var id string
	err = tx.QueryRow(ctx, "INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,$4,$5,$6) RETURNING student_id::text", input.UserID, v.IndexNo, v.FullName, v.Faculty, v.Year, v.ContactPhone).Scan(&id)
	if err != nil {
		return sdomain.Profile{}, dbError(err)
	}
	if err = guardians(ctx, tx, id, v.Guardians); err != nil {
		return sdomain.Profile{}, err
	}
	result, err := get(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if tx.Commit(ctx) != nil {
		return sdomain.Profile{}, sdomain.ErrUnavailable
	}
	return result, nil
}

func (s *Store) Update(ctx context.Context, actor, id string, input sdto.Details) (sdomain.Profile, error) {
	v := input
	tx, err := s.begin(ctx, actor)
	if err != nil {
		return sdomain.Profile{}, err
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, "UPDATE hostelhive.students SET index_no=$2,full_name=$3,faculty=$4,year=$5,contact_phone=$6 WHERE student_id=$1 AND deleted_at IS NULL", id, v.IndexNo, v.FullName, v.Faculty, v.Year, v.ContactPhone)
	if err != nil {
		return sdomain.Profile{}, dbError(err)
	}
	if tag.RowsAffected() != 1 {
		return sdomain.Profile{}, sdomain.ErrNotFound
	}
	if _, err = tx.Exec(ctx, "DELETE FROM hostelhive.guardians WHERE student_id=$1", id); err != nil {
		return sdomain.Profile{}, sdomain.ErrUnavailable
	}
	if err = guardians(ctx, tx, id, v.Guardians); err != nil {
		return sdomain.Profile{}, err
	}
	result, err := get(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if tx.Commit(ctx) != nil {
		return sdomain.Profile{}, sdomain.ErrUnavailable
	}
	return result, nil
}

func (s *Store) Delete(ctx context.Context, actor, id string) error {
	if !validation.UUID(id) {
		return sdomain.ErrInvalid
	}
	tx, err := s.begin(ctx, actor)
	if err != nil {
		return err
	}
	defer rollback(tx)
	// Queue private image deletion atomically with profile removal.
	if _, err = tx.Exec(ctx, `UPDATE hostelhive.student_image_objects SET cleanup_at=clock_timestamp()
 WHERE object_key=(SELECT profile_image_key FROM hostelhive.students WHERE student_id=$1)`, id); err != nil {
		return sdomain.ErrUnavailable
	}
	// Keep the row and guardian relationships available for historical references.
	tag, err := tx.Exec(ctx, `UPDATE hostelhive.students SET profile_image_key=NULL,deleted_at=COALESCE(deleted_at,clock_timestamp()),
 deleted_by=COALESCE(deleted_by,(SELECT user_id FROM hostelhive.users WHERE firebase_uid=$2)) WHERE student_id=$1`, id, actor)
	if err != nil {
		return sdomain.ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return sdomain.ErrNotFound
	}
	if tx.Commit(ctx) != nil {
		return sdomain.ErrUnavailable
	}
	return nil
}
