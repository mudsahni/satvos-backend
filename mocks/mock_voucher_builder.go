package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
)

// MockVoucherBuilder is a mock implementation of service.VoucherBuilderService.
type MockVoucherBuilder struct {
	mock.Mock
}

func (m *MockVoucherBuilder) Build(ctx context.Context, tenantID uuid.UUID, doc *domain.Document) (*domain.VoucherDef, error) {
	args := m.Called(ctx, tenantID, doc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.VoucherDef), args.Error(1)
}
