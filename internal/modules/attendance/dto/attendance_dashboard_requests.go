package dto

// PresenceFilter represents validated dashboard query parameters.
type PresenceFilter struct {
	Status string
	Query  string
	Limit  int
	Offset int
}
