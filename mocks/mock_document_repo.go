package mocks

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// MockDocumentRepo is a mock implementation of port.DocumentRepository.
type MockDocumentRepo struct {
	mock.Mock
}

func (m *MockDocumentRepo) Create(ctx context.Context, doc *domain.Document) error {
	args := m.Called(ctx, doc)
	return args.Error(0)
}

func (m *MockDocumentRepo) GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*domain.Document, error) {
	args := m.Called(ctx, tenantID, docID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentRepo) GetByFileID(ctx context.Context, tenantID, fileID uuid.UUID) (*domain.Document, error) {
	args := m.Called(ctx, tenantID, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentRepo) ListByCollection(ctx context.Context, tenantID, collectionID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, collectionID, filters, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentRepo) ListByTenant(ctx context.Context, tenantID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, filters, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentRepo) ListByUserCollections(ctx context.Context, tenantID, userID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, userID, filters, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentRepo) ListByServiceAccountCollections(ctx context.Context, tenantID, saID uuid.UUID, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, saID, filters, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentRepo) UpdateStructuredData(ctx context.Context, doc *domain.Document) error {
	args := m.Called(ctx, doc)
	return args.Error(0)
}

func (m *MockDocumentRepo) UpdateReviewStatus(ctx context.Context, doc *domain.Document) error {
	args := m.Called(ctx, doc)
	return args.Error(0)
}

func (m *MockDocumentRepo) UpdateAssignment(ctx context.Context, doc *domain.Document) error {
	args := m.Called(ctx, doc)
	return args.Error(0)
}

func (m *MockDocumentRepo) UpdateValidationResults(ctx context.Context, doc *domain.Document) error {
	args := m.Called(ctx, doc)
	return args.Error(0)
}

func (m *MockDocumentRepo) ListReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, userID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentRepo) ClaimQueued(ctx context.Context, limit int) ([]domain.Document, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Document), args.Error(1)
}

func (m *MockDocumentRepo) ResetStaleProcessing(ctx context.Context, staleBefore time.Time) (int, error) {
	args := m.Called(ctx, staleBefore)
	return args.Int(0), args.Error(1)
}

func (m *MockDocumentRepo) Delete(ctx context.Context, tenantID, docID uuid.UUID) error {
	args := m.Called(ctx, tenantID, docID)
	return args.Error(0)
}
