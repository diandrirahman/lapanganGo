package email

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"lapangango-api/internal/config"
)

func TestSMTPServiceTransportModeHonorsTLSConfig(t *testing.T) {
	tests := []struct {
		name     string
		useTLS   bool
		port     int
		expected smtpTransportMode
	}{
		{
			name:     "plain smtp for local mailpit",
			useTLS:   false,
			port:     1025,
			expected: smtpTransportPlain,
		},
		{
			name:     "starttls for submission port",
			useTLS:   true,
			port:     587,
			expected: smtpTransportStartTLS,
		},
		{
			name:     "implicit tls for smtps port",
			useTLS:   true,
			port:     465,
			expected: smtpTransportImplicitTLS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewSMTPService(config.Config{
				EmailDeliveryEnabled: true,
				SMTPUseTLS:           tt.useTLS,
				SMTPPort:             tt.port,
			})

			if got := service.transportMode(); got != tt.expected {
				t.Fatalf("transportMode() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNoopServiceDoesNotLogInviteOrResetURLs(t *testing.T) {
	var buf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
	})

	service := NewNoopService()
	err := service.SendStaffInvite(context.Background(), InviteEmailParams{
		ToEmail:   "staff@example.com",
		StaffName: "Staff",
		InviteURL: "https://app.example.com/staff/setup-password?token=invite-secret",
	})
	if err != nil {
		t.Fatalf("SendStaffInvite() error = %v", err)
	}

	err = service.SendStaffPasswordReset(context.Background(), ResetEmailParams{
		ToEmail:   "staff@example.com",
		StaffName: "Staff",
		ResetURL:  "https://app.example.com/staff/setup-password?token=reset-secret",
	})
	if err != nil {
		t.Fatalf("SendStaffPasswordReset() error = %v", err)
	}

	output := buf.String()
	for _, forbidden := range []string{"invite-secret", "reset-secret", "setup-password", "https://app.example.com"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("NoopService log leaked %q in output: %s", forbidden, output)
		}
	}
}

func TestNoopServiceDisabled(t *testing.T) {
	if NewNoopService().Enabled() {
		t.Fatal("NoopService.Enabled() = true, want false")
	}
}

func TestMain(m *testing.M) {
	log.SetFlags(0)
	os.Exit(m.Run())
}
