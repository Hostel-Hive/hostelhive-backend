package domain

// Restored is false when an eligible profile was already current. An exact
// retry never creates another audit record or changes the profile timestamp.
type RestorationResult struct {
	Student  Profile `json:"student"`
	Restored bool    `json:"restored"`
}
