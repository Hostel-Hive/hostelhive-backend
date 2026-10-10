package domain

type GuardianInput struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	ContactPhone string `json:"contact_phone"`
}

type Guardian struct {
	GuardianID string `json:"guardian_id"`
	GuardianInput
}
