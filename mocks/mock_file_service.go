package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
)

// MockFileService is a mock implementation of service.FileService.
type MockFileService struct {
	mock.Mock
}

func (m *MockFileService) Upload(ctx context.Context, input service.FileUploadInput) (*domain.FileMeta, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.FileMeta), args.Error(1)
}

func (m *MockFileService) GetByID(ctx context.Context, tenantID, fileID uuid.UUID) (*domain.FileMeta, error) {
	args := m.Called(ctx, tenantID, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.FileMeta), args.Error(1)
}

func (m *MockFileService) GetByIDForUser(ctx context.Context, tenantID, fileID, userID uuid.UUID, role domain.UserRole) (*domain.FileMeta, error) {
	args := m.Called(ctx, tenantID, fileID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.FileMeta), args.Error(1)
}

func (m *MockFileService) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error) {
	args := m.Called(ctx, tenantID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.FileMeta), args.Int(1), args.Error(2)
}

func (m *MockFileService) ListByUploader(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error) {
	args := m.Called(ctx, tenantID, userID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.FileMeta), args.Int(1), args.Error(2)
}

func (m *MockFileService) GetDownloadURL(ctx context.Context, tenantID, fileID uuid.UUID) (string, error) {
	args := m.Called(ctx, tenantID, fileID)
	return args.String(0), args.Error(1)
}

func (m *MockFileService) Delete(ctx context.Context, tenantID, fileID uuid.UUID) error {
	args := m.Called(ctx, tenantID, fileID)
	return args.Error(0)
}
