package blockedslots

import "time"

type CreateBlockedSlotRequest struct {
	StartAt string `json:"start_at" binding:"required"`
	EndAt   string `json:"end_at" binding:"required"`
	Reason  string `json:"reason" binding:"omitempty,max=180"`
}

type BlockedSlotResponse struct {
	ID        string    `json:"id"`
	CourtID   string    `json:"court_id"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
	Reason    *string   `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
