package commercialterms

import (
	"errors"
	"strings"
	"time"
)

type CommercialTerm struct {
	ID               string     `json:"id"`
	OwnerProfileID   *string    `json:"owner_profile_id"`
	ScopeKey         string     `json:"scope_key"`
	Label            string     `json:"label"`
	Phase            string     `json:"phase"`
	FinanceMode      string     `json:"finance_mode"`
	CollectionMethod string     `json:"collection_method"`
	CommissionBps    int        `json:"commission_bps"`
	ValidFrom        time.Time  `json:"valid_from"`
	ValidUntil       *time.Time `json:"valid_until"`
	SupersedesID     *string    `json:"supersedes_id"`
	CreatedByUserID  string     `json:"created_by_user_id"`
	CreatedAt        time.Time  `json:"created_at"`
	Status           string     `json:"status"` // CURRENT, SCHEDULED, HISTORICAL
}

type GetTermsQuery struct {
	Page           int    `form:"page" binding:"omitempty,min=1"`
	Limit          int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Scope          string `form:"scope" binding:"omitempty,oneof=GLOBAL OWNER ALL"`
	OwnerProfileID string `form:"owner_profile_id" binding:"omitempty,uuid"`
	Status         string `form:"status" binding:"omitempty,oneof=CURRENT SCHEDULED HISTORICAL"`
}

type PaginatedTermsResponse struct {
	Data       []CommercialTerm `json:"data"`
	TotalItems int              `json:"total_items"`
	TotalPages int              `json:"total_pages"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
}

type PreviewRequest struct {
	CommissionBps    int        `json:"commission_bps"`
	ValidFrom        time.Time  `json:"valid_from" binding:"required"`
	ValidUntil       *time.Time `json:"valid_until"`
	FinanceMode      string     `json:"finance_mode" binding:"required"`
	CollectionMethod string     `json:"collection_method" binding:"required"`
}

func (r *PreviewRequest) Validate() error {
	if r.CommissionBps < 0 || r.CommissionBps > 3000 {
		return errors.New("commission_bps must be between 0 and 3000")
	}
	if r.ValidUntil != nil && !r.ValidUntil.After(r.ValidFrom) {
		return errors.New("valid_until must be strictly greater than valid_from")
	}
	if r.FinanceMode != "SIMULATION" {
		return errors.New("finance_mode must be SIMULATION")
	}
	if r.CollectionMethod != "NONE" {
		return errors.New("collection_method must be NONE")
	}
	return nil
}

type PreviewScenario struct {
	BookingAmountInt64        int64 `json:"booking_amount_rupiah"`
	CommissionBps             int   `json:"commission_bps"`
	ProjectedCommissionRupiah int64 `json:"projected_commission_rupiah"`
	ProjectedOwnerNetRupiah   int64 `json:"projected_owner_net_rupiah"`
}

type PreviewResponse struct {
	FinanceMode      string            `json:"finance_mode"`
	CollectionMethod string            `json:"collection_method"`
	Scenarios        []PreviewScenario `json:"scenarios"`
}

type CreateTermRequest struct {
	OwnerProfileID   *string   `json:"owner_profile_id" binding:"omitempty,uuid"`
	Label            string    `json:"label" binding:"required"`
	Phase            string    `json:"phase" binding:"required"`
	FinanceMode      string    `json:"finance_mode" binding:"required"`
	CollectionMethod string    `json:"collection_method" binding:"required"`
	CommissionBps    *int      `json:"commission_bps" binding:"required"`
	ValidFrom        time.Time `json:"valid_from" binding:"required"`
}

func (r *CreateTermRequest) Validate() error {
	r.Label = strings.TrimSpace(r.Label)
	if r.Label == "" {
		return errors.New("label cannot be empty")
	}
	if len(r.Label) > 120 {
		return errors.New("label cannot exceed 120 characters")
	}

	if r.Phase != "TRIAL" && r.Phase != "INTRODUCTORY" && r.Phase != "STANDARD" && r.Phase != "CUSTOM" {
		return errors.New("invalid phase")
	}

	if r.FinanceMode != "SIMULATION" && r.FinanceMode != "LIVE" {
		return errors.New("finance_mode must be SIMULATION or LIVE")
	}

	if r.CollectionMethod != "NONE" {
		return errors.New("collection_method must be NONE")
	}

	if r.CommissionBps == nil {
		return errors.New("commission_bps is required")
	}

	if *r.CommissionBps < 0 || *r.CommissionBps > 3000 {
		return errors.New("commission_bps must be between 0 and 3000")
	}

	return nil
}
