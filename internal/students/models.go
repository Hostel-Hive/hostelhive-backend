// Package students implements administrator-managed student profiles (UC003).
package students

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("invalid input")
	ErrNotFound    = errors.New("student not found")
	ErrConflict    = errors.New("duplicate student")
	ErrAccount     = errors.New("active student account required")
	ErrForbidden   = errors.New("forbidden")
	ErrUnavailable = errors.New("student profiles unavailable")
)

type GuardianInput struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	ContactPhone string `json:"contact_phone"`
}
type Guardian struct {
	GuardianID string `json:"guardian_id"`
	GuardianInput
}
type Details struct {
	IndexNo      string          `json:"index_no"`
	FullName     string          `json:"full_name"`
	Faculty      string          `json:"faculty"`
	Year         int             `json:"year"`
	ContactPhone string          `json:"contact_phone"`
	Guardians    []GuardianInput `json:"guardians"`
}
type CreateInput struct {
	UserID string `json:"user_id"`
	Details
}
type Profile struct {
	StudentID     string     `json:"student_id"`
	UserID        string     `json:"user_id"`
	IndexNo       string     `json:"index_no"`
	FullName      string     `json:"full_name"`
	Faculty       string     `json:"faculty"`
	Year          int        `json:"year"`
	ContactPhone  string     `json:"contact_phone"`
	Email         string     `json:"email"`
	AccountActive bool       `json:"account_active"`
	Guardians     []Guardian `json:"guardians,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
type Filter struct {
	Limit, Offset, Year int
	Q, Faculty          string
}
type Page struct {
	Students []Profile `json:"students"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
	HasMore  bool      `json:"has_more"`
}

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var phonePattern = regexp.MustCompile(`^[+()0-9 -]{7,32}$`)

func text(value string, max int) bool {
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > max {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func phone(value string) bool {
	if !phonePattern.MatchString(value) {
		return false
	}
	n := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n >= 7
}
func Normalize(v Details) (Details, error) {
	v.IndexNo = strings.TrimSpace(v.IndexNo)
	v.FullName = strings.TrimSpace(v.FullName)
	v.Faculty = strings.TrimSpace(v.Faculty)
	v.ContactPhone = strings.TrimSpace(v.ContactPhone)
	if !text(v.IndexNo, 64) || !text(v.FullName, 200) || !text(v.Faculty, 120) || v.Year < 1 || v.Year > 10 || !phone(v.ContactPhone) || len(v.Guardians) < 1 || len(v.Guardians) > 10 {
		return Details{}, ErrInvalid
	}
	v.Guardians = append([]GuardianInput(nil), v.Guardians...)
	for n, g := range v.Guardians {
		g.Name = strings.TrimSpace(g.Name)
		g.Relationship = strings.TrimSpace(g.Relationship)
		g.ContactPhone = strings.TrimSpace(g.ContactPhone)
		if !text(g.Name, 200) || !text(g.Relationship, 80) || !phone(g.ContactPhone) {
			return Details{}, ErrInvalid
		}
		v.Guardians[n] = g
	}
	return v, nil
}
