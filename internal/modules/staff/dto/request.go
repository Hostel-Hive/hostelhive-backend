package dto

type Details struct {
	FullName    string `json:"full_name"`
	Designation string `json:"designation"`
}

// SelfDetails deliberately excludes designation, role and target account ID.
type SelfDetails struct {
	FullName string `json:"full_name"`
}
