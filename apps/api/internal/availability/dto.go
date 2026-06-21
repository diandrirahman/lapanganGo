package availability

import "time"

type SlotResponse struct {
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
	Status  string    `json:"status"`
}

type AvailabilityResponse struct {
	CourtID string         `json:"court_id"`
	Date    string         `json:"date"`
	Status  string         `json:"status"`
	Slots   []SlotResponse `json:"slots"`
}
