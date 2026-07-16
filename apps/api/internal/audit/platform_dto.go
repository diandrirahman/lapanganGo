package audit

import (
	"encoding/hex"
	"errors"
	"math"
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
		"reason":              true,
		"request_fingerprint": true,
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
				if !ok || (v != "TRIAL" && v != "INTRODUCTORY" && v != "STANDARD" && v != "CUSTOM") {
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
			case "request_fingerprint":
				v, ok := val.(string)
				if !ok || len(v) != 64 {
					return errors.New("request_fingerprint must be a 64-character SHA-256 hex string")
				}
				if _, err := hex.DecodeString(v); err != nil {
					return errors.New("request_fingerprint must be valid hexadecimal")
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

// SanitizePlatformAuditMetadata projects platform audit metadata onto the
// allowlisted, scalar fields that are safe for an administrative read API.
// Invalid or unknown values are omitted rather than being returned verbatim.
func SanitizePlatformAuditMetadata(action string, metadata map[string]any) map[string]any {
	out := make(map[string]any)
	allowedKeys := allowedMetadataKeysPerAction[action]
	for key, value := range metadata {
		if !allowedKeys[key] || key == "request_fingerprint" {
			continue
		}

		switch key {
		case "commission_bps":
			if bps, ok := scalarCommissionBPS(value); ok {
				out[key] = bps
			}
		case "label":
			if value, ok := value.(string); ok && len(value) <= 200 && !containsSecret(value) {
				out[key] = value
			}
		case "valid_from":
			if value, ok := value.(string); ok && !containsSecret(value) {
				out[key] = value
			}
		case "phase":
			if value, ok := value.(string); ok && (value == "TRIAL" || value == "INTRODUCTORY" || value == "STANDARD" || value == "CUSTOM") {
				out[key] = value
			}
		case "superseded_term_id", "new_term_id":
			if value, ok := value.(string); ok {
				if _, err := uuid.Parse(value); err == nil {
					out[key] = value
				}
			}
		case "reason":
			if value, ok := value.(string); ok && (value == "LIVE_NOT_ALLOWED" || value == "BPS_OUT_OF_BOUNDS" || value == "OVERLAP" || value == "INVALID_TIME" || value == "VALIDATION_ERROR") {
				out[key] = value
			}
		}
	}
	return out
}

// SanitizeAuditMetadata applies a conservative scalar-only projection to
// legacy owner audit metadata before it crosses the admin read boundary.
func SanitizeAuditMetadata(metadata map[string]any) map[string]any {
	out := make(map[string]any)
	for key, value := range metadata {
		if containsSecret(key) || !safeAuditMetadataScalar(value) {
			continue
		}
		out[key] = value
	}
	return out
}

func safeAuditMetadataScalar(value any) bool {
	switch value := value.(type) {
	case nil, bool, int, int64:
		return value != nil
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	case string:
		return len(value) <= 500 && !containsSecret(value)
	default:
		return false
	}
}

func scalarCommissionBPS(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		if value >= 0 && value <= 3000 {
			return value, true
		}
	case int64:
		if value >= 0 && value <= 3000 {
			return int(value), true
		}
	case float64:
		if value >= 0 && value <= 3000 && value == math.Trunc(value) {
			return int(value), true
		}
	}
	return 0, false
}
