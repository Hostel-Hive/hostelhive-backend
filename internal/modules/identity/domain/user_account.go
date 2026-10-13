package domain

type Account struct {
	FullName    string `json:"full_name,omitempty"`
	Designation string `json:"designation,omitempty"`
	UserID      string `json:"user_id"`
	FirebaseUID string `json:"firebase_uid"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
}
