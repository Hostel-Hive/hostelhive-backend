package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ActiveFaculty resolves only the current selectable name. Historical aliases
// preserve audience identity but are not accepted for new registrations/notices.
// Call inside the caller's authorized transaction, holding the users table lock.
func ActiveFaculty(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	var canonical string
	e := tx.QueryRow(ctx, `SELECT name FROM hostelhive.faculties WHERE lower(name)=lower(btrim($1)) AND is_active FOR SHARE`, name).Scan(&canonical)
	return canonical, e
}
