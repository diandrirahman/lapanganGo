package audit

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

const (
	ActionPlatformCommercialTermCreated      = "PLATFORM_COMMERCIAL_TERM_CREATED"
	ActionPlatformCommercialTermSuperseded   = "PLATFORM_COMMERCIAL_TERM_SUPERSEDED"
	ActionPlatformCommercialTermLiveRejected = "PLATFORM_COMMERCIAL_TERM_LIVE_REJECTED"

	EntityPlatformCommercialTerm = "PLATFORM_COMMERCIAL_TERM"
)

var allowedPlatformActions = map[string]bool{
	ActionPlatformCommercialTermCreated:      true,
	ActionPlatformCommercialTermSuperseded:   true,
	ActionPlatformCommercialTermLiveRejected: true,
}

var allowedPlatformEntities = map[string]bool{
	EntityPlatformCommercialTerm: true,
}

var allowedMetadataKeysPerAction = map[string]map[string]bool{
	ActionPlatformCommercialTermCreated: {
		"commission_bps": true,
		"label":          true,
		"valid_from":     true,
		"phase":          true,
	},
	ActionPlatformCommercialTermSuperseded: {
		"superseded_term_id": true,
		"new_term_id":        true,
	},
	ActionPlatformCommercialTermLiveRejected: {
		"reason": true,
	},
}

var forbiddenMetadataKeys = []string{
	"secret", "token", "password", "authorization", "credential", "payload", "pii", "bearer",
}

type CreatePlatformAuditLogParams struct {
	ActorUserID    *string
	ActorRole      string
	Action         string
	EntityType     string
	EntityID       *string
	OwnerProfileID *string
	VenueID        *string
	CorrelationID  *string
	Metadata       map[string]any
	IPAddress      *string
	UserAgent      *string
}

func (p *CreatePlatformAuditLogParams) Validate() error {
	if !allowedPlatformActions[p.Action] {
		return errors.New("invalid platform audit action")
	}
	if !allowedPlatformEntities[p.EntityType] {
		return errors.New("invalid platform audit entity type")
	}
	if p.ActorRole == "" {
		return errors.New("actor role is required")
	}

	allowedKeys := allowedMetadataKeysPerAction[p.Action]

	if p.Metadata != nil {
		for key, val := range p.Metadata {
			if !allowedKeys[key] {
				return errors.New("metadata key not allowed for this action: " + key)
			}

			// Value validation
			switch key {
			case "commission_bps":
				switch v := val.(type) {
				case float64:
					if v < 0 || v > 3000 {
						return errors.New("commission_bps out of bounds")
					}
				case int:
					if v < 0 || v > 3000 {
						return errors.New("commission_bps out of bounds")
					}
				default:
					return errors.New("commission_bps must be a number")
				}
			case "label":
				v, ok := val.(string)
				if !ok || len(v) > 200 {
					return errors.New("label must be string <= 200 chars")
				}
				if containsSecret(v) {
					return errors.New("label contains sensitive information")
				}
			case "valid_from":
				v, ok := val.(string)
				if !ok {
					return errors.New("valid_from must be a date string")
				}
				if containsSecret(v) {
					return errors.New("valid_from contains sensitive information")
				}
			case "phase":
				v, ok := val.(string)
				if !ok || (v != "TRIAL" && v != "PRODUCTION") {
					return errors.New("phase must be valid enum")
				}
			case "superseded_term_id", "new_term_id":
				v, ok := val.(string)
				if !ok {
					return errors.New(key + " must be a UUID string")
				}
				if _, err := uuid.Parse(v); err != nil {
					return errors.New(key + " must be a valid UUID")
				}
			case "reason":
				v, ok := val.(string)
				if !ok {
					return errors.New("reason must be string")
				}
				// Strict enum for reason
				if v != "LIVE_NOT_ALLOWED" && v != "BPS_OUT_OF_BOUNDS" && v != "OVERLAP" && v != "INVALID_TIME" && v != "VALIDATION_ERROR" {
					return errors.New("reason must be an allowed code")
				}
			}
		}
	} else {
		p.Metadata = make(map[string]any)
	}

	return nil
}

func containsSecret(s string) bool {
	lower := strings.ToLower(s)
	for _, f := range forbiddenMetadataKeys {
		if strings.Contains(lower, f) {
			return true
		}
	}
	return false
}
