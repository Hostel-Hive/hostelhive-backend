package dto

type CheckoutInput struct {
	RequestID         string `json:"request_id"`
	Reason            string `json:"reason"`
	Notes             string `json:"notes"`
	DeactivateAccount *bool  `json:"deactivate_account"`
	Confirmed         bool   `json:"confirmed"`
}
