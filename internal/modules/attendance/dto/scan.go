package dto

type Scan struct {
	EventID string `json:"event_id"`
	QR      string `json:"qr"`
}

type SetDirection struct {
	Direction string `json:"direction"`
}
