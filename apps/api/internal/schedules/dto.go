package schedules

import "time"

type OperatingHourRequest struct {
	DayOfWeek *int    `json:"day_of_week" binding:"required,gte=0,lte=6"`
	OpenTime  *string `json:"open_time"`
	CloseTime *string `json:"close_time"`
	IsClosed  bool    `json:"is_closed"`
}

type ReplaceOperatingHoursRequest struct {
	Days []OperatingHourRequest `json:"days" binding:"required"`
}

type OperatingHourResponse struct {
	ID        string    `json:"id"`
	CourtID   string    `json:"court_id"`
	DayOfWeek int       `json:"day_of_week"`
	OpenTime  *string   `json:"open_time"`
	CloseTime *string   `json:"close_time"`
	IsClosed  bool      `json:"is_closed"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
