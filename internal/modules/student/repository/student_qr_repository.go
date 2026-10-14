package repository

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	sdomain "github.com/Hostel-Hive/hostelhive-backend/internal/modules/student/domain"
	"github.com/Hostel-Hive/hostelhive-backend/internal/shared/validation"
	"github.com/jackc/pgx/v5"
)

func (s *Store) QR(ctx context.Context, actor, studentID, candidate string) (string, error) {
	decoded, err := hex.DecodeString(candidate)
	if err != nil || len(decoded) != 32 || candidate != strings.ToLower(candidate) ||
		(studentID != "" && !validation.UUID(studentID)) {
		return "", sdomain.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", sdomain.ErrUnavailable
	}
	defer rollback(tx)
	// Coordinate with role/deactivation writes, then recheck the actor. The
	// student row lock also serializes lazy issuance against profile archival.
	if _, err = tx.Exec(ctx, "LOCK TABLE hostelhive.users IN SHARE MODE"); err != nil {
		return "", sdomain.ErrUnavailable
	}
	var userID, role string
	err = tx.QueryRow(ctx, "SELECT user_id::text,role FROM hostelhive.users WHERE firebase_uid=$1 AND is_active", actor).Scan(&userID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", sdomain.ErrForbidden
	}
	if err != nil {
		return "", sdomain.ErrUnavailable
	}
	if (studentID == "" && role != "student") || (studentID != "" && role != "admin" && role != "warden") {
		return "", sdomain.ErrForbidden
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT s.student_id::text FROM hostelhive.students s
 JOIN hostelhive.users u USING(user_id)
 WHERE s.deleted_at IS NULL AND u.is_active AND u.role='student'
 AND (($1::text='' AND s.user_id=$2::uuid) OR s.student_id::text=$1)
 FOR UPDATE OF s`, strings.ToLower(studentID), userID).Scan(&id)
	if err != nil {
		return "", dbError(err)
	}
	var token string
	err = tx.QueryRow(ctx, "SELECT token FROM hostelhive.student_qr WHERE student_id=$1", id).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, "INSERT INTO hostelhive.student_qr(student_id,token) VALUES($1,$2) RETURNING token", id, candidate).Scan(&token)
	}
	if err != nil {
		return "", sdomain.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return "", sdomain.ErrUnavailable
	}
	return token, nil
}
