package domain

type Reservation struct {
	Key, RequesterUID, FirebaseUID, Email, Role string
	Completed                                   bool
}
