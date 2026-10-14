package domain

import "time"

type Profile struct {
	ProfileImageURL string     `json:"profile_image_url,omitempty"`
	StudentID       string     `json:"student_id"`
	UserID          string     `json:"user_id"`
	IndexNo         string     `json:"index_no"`
	FullName        string     `json:"full_name"`
	Faculty         string     `json:"faculty"`
	Year            int        `json:"year"`
	ContactPhone    string     `json:"contact_phone"`
	Email           string     `json:"email"`
	AccountActive   bool       `json:"account_active"`
	Guardians       []Guardian `json:"guardians,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
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
