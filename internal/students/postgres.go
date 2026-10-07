package students

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const profileColumns = "s.student_id::text,s.user_id::text,s.index_no,s.full_name,s.faculty,s.year,s.contact_phone,u.email,u.is_active,s.created_at,s.updated_at"

func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && (p.Code == "23505") {
		return ErrConflict
	}
	return ErrUnavailable
}
func scan(row pgx.Row) (Profile, error) {
	var v Profile
	err := row.Scan(&v.StudentID, &v.UserID, &v.IndexNo, &v.FullName, &v.Faculty, &v.Year, &v.ContactPhone, &v.Email, &v.AccountActive, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, dbError(err)
	}
	return v, nil
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func get(ctx context.Context, q querier, id string) (Profile, error) {
	v, err := scan(q.QueryRow(ctx, "SELECT "+profileColumns+" FROM hostelhive.students s JOIN hostelhive.users u USING(user_id) WHERE s.student_id=$1 AND s.deleted_at IS NULL", id))
	if err != nil {
		return v, err
	}
	rows, err := q.Query(ctx, "SELECT guardian_id::text,name,relationship,contact_phone FROM hostelhive.guardians WHERE student_id=$1 ORDER BY guardian_id", id)
	if err != nil {
		return v, ErrUnavailable
	}
	defer rows.Close()
	v.Guardians = []Guardian{}
	for rows.Next() {
		var g Guardian
		if rows.Scan(&g.GuardianID, &g.Name, &g.Relationship, &g.ContactPhone) != nil {
			return v, ErrUnavailable
		}
		v.Guardians = append(v.Guardians, g)
	}
	if rows.Err() != nil {
		return v, ErrUnavailable
	}
	return v, nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) Get(ctx context.Context, id string) (Profile, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Profile{}, ErrUnavailable
	}
	defer rollback(tx)
	v, err := get(ctx, tx, id)
	if err != nil {
		return v, err
	}
	if tx.Commit(ctx) != nil {
		return Profile{}, ErrUnavailable
	}
	return v, nil
}
func (s *Store) List(ctx context.Context, f Filter) (Page, error) {
	page := Page{Students: []Profile{}, Limit: f.Limit, Offset: f.Offset}
	rows, err := s.pool.Query(ctx, "SELECT "+profileColumns+` FROM hostelhive.students s JOIN hostelhive.users u USING(user_id)
 WHERE s.deleted_at IS NULL AND ($1::text='' OR strpos(lower(s.index_no),lower($1))>0 OR strpos(lower(s.full_name),lower($1))>0)
 AND ($2::text='' OR lower(s.faculty)=lower($2)) AND ($3::int=0 OR s.year=$3)
 ORDER BY s.created_at,s.student_id LIMIT $4 OFFSET $5`, f.Q, f.Faculty, f.Year, f.Limit+1, f.Offset)
	if err != nil {
		return page, ErrUnavailable
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
		return page, ErrUnavailable
	}
	if len(page.Students) > f.Limit {
		page.HasMore = true
		page.Students = page.Students[:f.Limit]
	}
	return page, nil
}
func (s *Store) begin(ctx context.Context, actor string) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		rollback(tx)
		return nil, ErrUnavailable
	}
	var ok bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE firebase_uid=$1 AND role='admin' AND is_active)", actor).Scan(&ok)
	if err != nil {
		rollback(tx)
		return nil, ErrUnavailable
	}
	if !ok {
		rollback(tx)
		return nil, ErrForbidden
	}
	return tx, nil
}
func guardians(ctx context.Context, tx pgx.Tx, id string, gs []GuardianInput) error {
	for _, g := range gs {
		if _, err := tx.Exec(ctx, "INSERT INTO hostelhive.guardians(student_id,name,relationship,contact_phone) VALUES($1,$2,$3,$4)", id, g.Name, g.Relationship, g.ContactPhone); err != nil {
			return dbError(err)
		}
	}
	return nil
}
func (s *Store) Create(ctx context.Context, actor string, input CreateInput) (Profile, error) {
	v, err := Normalize(input.Details)
	if err != nil || !uuid.MatchString(input.UserID) {
		return Profile{}, ErrInvalid
	}
	tx, err := s.begin(ctx, actor)
	if err != nil {
		return Profile{}, err
	}
	defer rollback(tx)
	var eligible bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hostelhive.users WHERE user_id=$1 AND role='student' AND is_active)", input.UserID).Scan(&eligible)
	if err != nil {
		return Profile{}, ErrUnavailable
	}
	if !eligible {
		return Profile{}, ErrAccount
	}
	var id string
	err = tx.QueryRow(ctx, "INSERT INTO hostelhive.students(user_id,index_no,full_name,faculty,year,contact_phone) VALUES($1,$2,$3,$4,$5,$6) RETURNING student_id::text", input.UserID, v.IndexNo, v.FullName, v.Faculty, v.Year, v.ContactPhone).Scan(&id)
	if err != nil {
		return Profile{}, dbError(err)
	}
	if err = guardians(ctx, tx, id, v.Guardians); err != nil {
		return Profile{}, err
	}
	result, err := get(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if tx.Commit(ctx) != nil {
		return Profile{}, ErrUnavailable
	}
	return result, nil
}
func (s *Store) Update(ctx context.Context, actor, id string, input Details) (Profile, error) {
	v, err := Normalize(input)
	if err != nil || !uuid.MatchString(id) {
		return Profile{}, ErrInvalid
	}
	tx, err := s.begin(ctx, actor)
	if err != nil {
		return Profile{}, err
	}
	defer rollback(tx)
	tag, err := tx.Exec(ctx, "UPDATE hostelhive.students SET index_no=$2,full_name=$3,faculty=$4,year=$5,contact_phone=$6 WHERE student_id=$1 AND deleted_at IS NULL", id, v.IndexNo, v.FullName, v.Faculty, v.Year, v.ContactPhone)
	if err != nil {
		return Profile{}, dbError(err)
	}
	if tag.RowsAffected() != 1 {
		return Profile{}, ErrNotFound
	}
	if _, err = tx.Exec(ctx, "DELETE FROM hostelhive.guardians WHERE student_id=$1", id); err != nil {
		return Profile{}, ErrUnavailable
	}
	if err = guardians(ctx, tx, id, v.Guardians); err != nil {
		return Profile{}, err
	}
	result, err := get(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if tx.Commit(ctx) != nil {
		return Profile{}, ErrUnavailable
	}
	return result, nil
}
func (s *Store) Delete(ctx context.Context, actor, id string) error {
	if !uuid.MatchString(id) {
		return ErrInvalid
	}
	tx, err := s.begin(ctx, actor)
	if err != nil {
		return err
	}
	defer rollback(tx)
	// Keep the row and guardian relationships available for historical references.
	tag, err := tx.Exec(ctx, `UPDATE hostelhive.students SET deleted_at=COALESCE(deleted_at,clock_timestamp()),
 deleted_by=COALESCE(deleted_by,(SELECT user_id FROM hostelhive.users WHERE firebase_uid=$2)) WHERE student_id=$1`, id, actor)
	if err != nil {
		return ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
