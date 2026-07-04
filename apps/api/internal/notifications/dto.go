package notifications

import "time"

type NotificationResponse struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Message    string     `json:"message"`
	EntityType *string    `json:"entity_type,omitempty"`
	EntityID   *string    `json:"entity_id,omitempty"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type NotificationListResponse struct {
	Data       []NotificationResponse `json:"data"`
	Page       int                    `json:"page"`
	Limit      int                    `json:"limit"`
	Total      int                    `json:"total"`
	TotalPages int                    `json:"total_pages"`
}

type UnreadCountResponse struct {
	Count int `json:"count"`
}
