package noop

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"satvos/internal/port"
)

type noopSender struct {
	frontendURL string
}

// NewNoopSender creates a no-op EmailSender that logs verification URLs to stdout.
func NewNoopSender(frontendURL string) port.EmailSender {
	return &noopSender{frontendURL: frontendURL}
}

func (s *noopSender) SendVerificationEmail(_ context.Context, toEmail, toName, verificationToken string) error {
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, url.QueryEscape(verificationToken))
	log.Printf("[NOOP EMAIL] Verification email for %s (%s): %s", toName, toEmail, verifyURL)
	return nil
}

func (s *noopSender) SendPasswordResetEmail(_ context.Context, toEmail, toName, resetToken string) error {
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, url.QueryEscape(resetToken))
	log.Printf("[NOOP EMAIL] Password reset for %s (%s): %s", toName, toEmail, resetURL)
	return nil
}

func (s *noopSender) SendInvitationEmail(_ context.Context, toEmail, toName, roleName, invitationToken string) error {
	inviteURL := fmt.Sprintf("%s/accept-invitation?token=%s", s.frontendURL, url.QueryEscape(invitationToken))
	log.Printf("[NOOP EMAIL] Invitation for %s (%s) as %s: %s", toName, toEmail, roleName, inviteURL)
	return nil
}
