package domain

type AccountPage struct {
	Users   []Account `json:"users"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"has_more"`
}

type AccountChange struct {
	Account           Account `json:"account"`
	RevocationPending bool    `json:"revocation_pending"`
}
