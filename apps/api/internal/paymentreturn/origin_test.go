package paymentreturn

import (
	"strings"
	"testing"
)

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "dns", value: "https://DEMO.EXAMPLE.TEST", want: "https://demo.example.test"},
		{name: "dns with port", value: "https://demo-1.example.test:443", want: "https://demo-1.example.test:443"},
		{name: "ipv4", value: "https://127.0.0.1:3000", want: "https://127.0.0.1:3000"},
		{name: "localhost", value: "https://localhost", want: "https://localhost"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeOrigin(test.value)
			if err != nil || got != test.want {
				t.Fatalf("NormalizeOrigin(%q) = %q, %v; want %q", test.value, got, err, test.want)
			}
		})
	}
}

func TestNormalizeOriginRejectsInvalidAuthority(t *testing.T) {
	invalid := []string{
		"",
		"http://demo.example.test",
		"https://user@demo.example.test",
		"https://demo.example.test/path",
		"https://demo.example.test?query=1",
		"https://demo.example.test#fragment",
		"https://-",
		"https://..example",
		"https://example..test",
		"https://-demo.example.test",
		"https://demo-.example.test",
		"https://demo_example.test",
		"https://démø.example.test",
		"https://127.0.0.01",
		"https://999.999.999.999",
		"https://1.2.3",
		"https://[::1]:3000",
		"https://demo.example.test:0",
		"https://demo.example.test:65536",
		"https://" + strings.Repeat("a", 64) + ".example.test",
		"https://" + strings.Repeat("a.", 127) + "a",
	}
	for _, value := range invalid {
		t.Run(value, func(t *testing.T) {
			if got, err := NormalizeOrigin(value); err == nil {
				t.Fatalf("invalid origin normalized to %q", got)
			}
		})
	}
}
