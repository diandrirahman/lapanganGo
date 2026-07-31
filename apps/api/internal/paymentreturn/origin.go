package paymentreturn

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalidOrigin = errors.New("invalid payment return origin")

// NormalizeOrigin accepts only a bare HTTPS origin with an ASCII DNS hostname
// or canonical IPv4 address and an optional port in the IANA range.
func NormalizeOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidOrigin
	}

	hostname := strings.ToLower(parsed.Hostname())
	if !validHostname(hostname) {
		return "", ErrInvalidOrigin
	}

	port := parsed.Port()
	if port == "" {
		// A colon without a parsed port is an IPv6 literal or malformed
		// authority. IPv6 is outside the frozen Phase 5B return contract.
		if strings.Contains(parsed.Host, ":") {
			return "", ErrInvalidOrigin
		}
		return "https://" + hostname, nil
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", ErrInvalidOrigin
	}
	return "https://" + hostname + ":" + strconv.Itoa(portNumber), nil
}

func validHostname(hostname string) bool {
	if hostname == "" || len(hostname) > 253 {
		return false
	}
	if address, err := netip.ParseAddr(hostname); err == nil {
		return address.Is4() && address.String() == hostname
	}
	if looksLikeIPv4(hostname) {
		return false
	}

	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || !isASCIIAlphanumeric(label[0]) ||
			!isASCIIAlphanumeric(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			if !isASCIIAlphanumeric(label[index]) && label[index] != '-' {
				return false
			}
		}
	}
	return true
}

func looksLikeIPv4(hostname string) bool {
	if !strings.Contains(hostname, ".") {
		return false
	}
	for index := range len(hostname) {
		if (hostname[index] < '0' || hostname[index] > '9') && hostname[index] != '.' {
			return false
		}
	}
	return true
}

func isASCIIAlphanumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9')
}
