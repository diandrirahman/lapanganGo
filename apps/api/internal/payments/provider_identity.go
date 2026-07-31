package payments

import "lapangango-api/internal/provideridentity"

// ValidProviderIdentity defines the storage-safe provider identity boundary.
// Provider identifiers are opaque, but they must satisfy the shared storage
// and outbox safety policy before any repository write or comparison.
func ValidProviderIdentity(value string, required bool) bool {
	return provideridentity.Valid(value, required)
}

func validOptionalProviderIdentity(value *string) bool {
	return value == nil || ValidProviderIdentity(*value, true)
}
