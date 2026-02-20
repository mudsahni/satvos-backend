package port

import "context"

// EmailSender defines the contract for sending emails.
type EmailSender interface {
	SendVerificationEmail(ctx context.Context, toEmail, toName, verificationToken string) error
	SendPasswordResetEmail(ctx context.Context, toEmail, toName, resetToken string) error
	SendInvitationEmail(ctx context.Context, toEmail, toName, roleName, invitationToken string) error
}
