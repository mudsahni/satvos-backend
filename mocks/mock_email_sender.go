package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockEmailSender is a mock implementation of port.EmailSender.
type MockEmailSender struct {
	mock.Mock
}

func (m *MockEmailSender) SendVerificationEmail(ctx context.Context, toEmail, toName, verificationToken string) error {
	args := m.Called(ctx, toEmail, toName, verificationToken)
	return args.Error(0)
}

func (m *MockEmailSender) SendPasswordResetEmail(ctx context.Context, toEmail, toName, resetToken string) error {
	args := m.Called(ctx, toEmail, toName, resetToken)
	return args.Error(0)
}

func (m *MockEmailSender) SendInvitationEmail(ctx context.Context, toEmail, toName, roleName, invitationToken string) error {
	args := m.Called(ctx, toEmail, toName, roleName, invitationToken)
	return args.Error(0)
}
