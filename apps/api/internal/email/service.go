package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/smtp"
	"time"

	"lapangango-api/internal/config"
)

type Service interface {
	SendStaffInvite(ctx context.Context, params InviteEmailParams) error
	SendStaffPasswordReset(ctx context.Context, params ResetEmailParams) error
	Enabled() bool
}

type InviteEmailParams struct {
	ToEmail   string
	StaffName string
	VenueName string // Optional
	InviteURL string
}

type ResetEmailParams struct {
	ToEmail   string
	StaffName string
	ResetURL  string
}

// SMTPService implements Service using standard net/smtp
type SMTPService struct {
	config config.Config
}

type smtpTransportMode string

const (
	smtpTransportPlain       smtpTransportMode = "plain"
	smtpTransportStartTLS    smtpTransportMode = "starttls"
	smtpTransportImplicitTLS smtpTransportMode = "implicit_tls"
)

func NewSMTPService(cfg config.Config) *SMTPService {
	return &SMTPService{config: cfg}
}

func (s *SMTPService) Enabled() bool {
	return s.config.EmailDeliveryEnabled
}

func (s *SMTPService) transportMode() smtpTransportMode {
	if !s.config.SMTPUseTLS {
		return smtpTransportPlain
	}
	if s.config.SMTPPort == 465 {
		return smtpTransportImplicitTLS
	}
	return smtpTransportStartTLS
}

func (s *SMTPService) SendStaffInvite(ctx context.Context, params InviteEmailParams) error {
	subject := "Undangan Staff LapangGo"
	if params.VenueName != "" {
		subject = fmt.Sprintf("Undangan Staff LapangGo untuk %s", params.VenueName)
	}

	body := GenerateStaffInviteBody(params)

	return s.sendMail(params.ToEmail, subject, body)
}

func (s *SMTPService) SendStaffPasswordReset(ctx context.Context, params ResetEmailParams) error {
	subject := "Reset Password Staff LapangGo"
	body := GenerateStaffPasswordResetBody(params)

	return s.sendMail(params.ToEmail, subject, body)
}

func (s *SMTPService) sendMail(to, subject, body string) error {
	from := s.config.SMTPFromEmail
	fromName := s.config.SMTPFromName

	var auth smtp.Auth
	if s.config.SMTPUsername != "" || s.config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.config.SMTPUsername, s.config.SMTPPassword, s.config.SMTPHost)
	}

	// RFC 822 format
	msg := bytes.NewBuffer(nil)
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", fromName, from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	if err := s.sendSMTP(addr, auth, from, []string{to}, msg.Bytes()); err != nil {
		log.Printf("Failed to send email to %s: %v", to, err)
		return err
	}

	log.Printf("Successfully sent email to %s", to)
	return nil
}

func (s *SMTPService) sendSMTP(addr string, auth smtp.Auth, from string, recipients []string, msg []byte) error {
	host := s.config.SMTPHost
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	var client *smtp.Client
	var err error

	switch s.transportMode() {
	case smtpTransportImplicitTLS:
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return err
		}
	case smtpTransportStartTLS, smtpTransportPlain:
		client, err = smtp.Dial(addr)
		if err != nil {
			return err
		}
		if s.transportMode() == smtpTransportStartTLS {
			if ok, _ := client.Extension("STARTTLS"); !ok {
				_ = client.Close()
				return fmt.Errorf("smtp server %s does not support STARTTLS", host)
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				_ = client.Close()
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported SMTP transport mode")
	}

	return sendWithSMTPClient(client, auth, from, recipients, msg)
}

func sendWithSMTPClient(client *smtp.Client, auth smtp.Auth, from string, recipients []string, msg []byte) error {
	defer client.Close()

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp server does not support AUTH")
		}
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	if err := client.Quit(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// NoopService is a dummy implementation when email delivery is disabled
type NoopService struct{}

func NewNoopService() *NoopService {
	return &NoopService{}
}

func (s *NoopService) Enabled() bool {
	return false
}

func (s *NoopService) SendStaffInvite(ctx context.Context, params InviteEmailParams) error {
	log.Printf("[Email Disabled] Skipping staff invite email to %s", params.ToEmail)
	return nil
}

func (s *NoopService) SendStaffPasswordReset(ctx context.Context, params ResetEmailParams) error {
	log.Printf("[Email Disabled] Skipping staff password reset email to %s", params.ToEmail)
	return nil
}
