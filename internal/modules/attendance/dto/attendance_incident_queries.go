package dto

type IncidentFilter struct {
	StudentID, Direction, IncidentType string
	Limit, Offset                      int
}
