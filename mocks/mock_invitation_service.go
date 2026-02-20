package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
)

// MockInvitationService is a mock implementation of service.InvitationService.
type MockInvitationService struct {
	mock.Mock
}

func (m *MockInvitationService) SendInvitation(ctx context.Context, user *domain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockInvitationService) ResendInvitation(ctx context.Context, tenantID, userID uuid.UUID) error {
	args := m.Called(ctx, tenantID, userID)
	return args.Error(0)
}

func (m *MockInvitationService) AcceptInvitation(ctx context.Context, input service.AcceptInvitationInput) (*service.AcceptInvitationOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.AcceptInvitationOutput), args.Error(1)
}
