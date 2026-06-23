package mabar

import "time"

type CreateOpenMatchRequest struct {
	Title          string  `json:"title" binding:"required,min=2,max=100"`
	Description    string  `json:"description" binding:"omitempty,max=500"`
	Level          string  `json:"level" binding:"required"`
	MaxPlayers     int     `json:"max_players" binding:"required,min=1"`
	PricePerPlayer float64 `json:"price_per_player" binding:"min=0"`
}

type OpenMatchResponse struct {
	ID             string    `json:"id"`
	BookingID      string    `json:"booking_id"`
	HostName       string    `json:"host_name"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	SportName      string    `json:"sport_name"`
	VenueName      string    `json:"venue_name"`
	CourtName      string    `json:"court_name"`
	MatchDate      string    `json:"match_date"`
	StartTime      string    `json:"start_time"`
	EndTime        string    `json:"end_time"`
	Level          string    `json:"level"`
	MaxPlayers     int       `json:"max_players"`
	JoinedCount    int       `json:"joined_count"`
	RemainingSlots int       `json:"remaining_slots"`
	PricePerPlayer float64   `json:"price_per_player"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ParticipantResponse struct {
	ID       string    `json:"id"`
	UserID   string    `json:"user_id"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	JoinedAt time.Time `json:"joined_at"`
}

type OpenMatchDetailResponse struct {
	OpenMatch    OpenMatchResponse     `json:"open_match"`
	Participants []ParticipantResponse `json:"participants"`
}
