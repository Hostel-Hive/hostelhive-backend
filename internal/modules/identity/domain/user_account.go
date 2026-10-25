package domain

type Account struct {
	StudentProfileID       *string `json:"student_profile_id,omitempty"`
	StudentProfileArchived *bool   `json:"student_profile_archived,omitempty"`
	StudentHasActiveBed    *bool   `json:"student_has_active_bed,omitempty"`
	SetupRequired          bool    `json:"setup_required,omitempty"`
	FullName               string  `json:"full_name,omitempty"`
	Designation            string  `json:"designation,omitempty"`
	UserID                 string  `json:"user_id"`
	FirebaseUID            string  `json:"firebase_uid"`
	Email                  string  `json:"email"`
	Role                   string  `json:"role"`
	IsActive               bool    `json:"is_active"`
}
