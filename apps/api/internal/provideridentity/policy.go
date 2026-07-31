package provideridentity

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBytes = 191

var canonicalUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var forbiddenPrefixes = map[string]struct{}{
	"acct": {}, "account": {}, "api": {}, "authorization": {}, "bank": {},
	"bearer": {}, "card": {}, "cvc": {}, "cvv": {}, "iban": {}, "key": {},
	"pan": {}, "password": {}, "payload": {}, "pk": {}, "raw": {}, "secret": {},
	"sk": {}, "token": {}, "xnd": {}, "provider": {}, "ref": {},
}

// Valid is the single provider identity boundary shared by payment storage
// and outbox digesting. Identifiers remain opaque, but must be bounded,
// printable, non-credential-shaped, and distinguishable from account digits
// or arbitrary free text.
func Valid(value string, required bool) bool {
	if value == "" {
		return !required
	}
	if len(value) > MaxBytes || !utf8.ValidString(value) ||
		value != strings.TrimSpace(value) || strings.Contains(value, "://") {
		return false
	}
	for _, char := range value {
		if !unicode.IsPrint(char) || unicode.IsSpace(char) {
			return false
		}
	}
	if canonicalUUIDPattern.MatchString(value) {
		return true
	}
	separatorIndex := strings.IndexAny(value, "-_:")
	if separatorIndex < 2 {
		return false
	}
	prefix := strings.ToLower(value[:separatorIndex])
	for _, char := range prefix {
		if char < 'a' || char > 'z' {
			return false
		}
	}
	if _, forbidden := forbiddenPrefixes[prefix]; forbidden {
		return false
	}
	suffix := value[separatorIndex+1:]
	if canonicalUUIDPattern.MatchString(strings.ToLower(suffix)) {
		return true
	}
	var hasLetter bool
	for _, char := range suffix {
		hasLetter = hasLetter || unicode.IsLetter(char)
	}
	return hasLetter
}
