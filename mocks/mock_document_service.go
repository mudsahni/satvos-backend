package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/service"
	"satvos/internal/validator"
)

// MockDocumentService is a mock implementation of service.DocumentService.
type MockDocumentService struct {
	mock.Mock
}

func (m *MockDocumentService) CreateAndParse(ctx context.Context, input *service.CreateDocumentInput) (*domain.Document, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentService) GetByID(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error) {
	args := m.Called(ctx, tenantID, docID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentService) GetByFileID(ctx context.Context, tenantID, fileID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error) {
	args := m.Called(ctx, tenantID, fileID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentService) ListByCollection(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, collectionID, userID, role, filters, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentService) ListByTenant(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, filters *port.DocumentListFilters, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, userID, role, filters, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentService) GetTagFacets(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, keys []string) (map[string][]port.TagFacet, error) {
	args := m.Called(ctx, tenantID, userID, role, keys)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]port.TagFacet), args.Error(1)
}

func (m *MockDocumentService) UpdateReview(ctx context.Context, input *service.UpdateReviewInput) (*domain.Document, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentService) AssignDocument(ctx context.Context, input *service.AssignDocumentInput) (*domain.Document, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentService) ListReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, userID, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentService) EditStructuredData(ctx context.Context, input *service.EditStructuredDataInput) (*domain.Document, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentService) RetryParse(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error) {
	args := m.Called(ctx, tenantID, docID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Document), args.Error(1)
}

func (m *MockDocumentService) ValidateDocument(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) error {
	args := m.Called(ctx, tenantID, docID, userID, role)
	return args.Error(0)
}

func (m *MockDocumentService) GetValidation(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*validator.ValidationResponse, error) {
	args := m.Called(ctx, tenantID, docID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*validator.ValidationResponse), args.Error(1)
}

func (m *MockDocumentService) Delete(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) error {
	args := m.Called(ctx, tenantID, docID, userID, role)
	return args.Error(0)
}

func (m *MockDocumentService) ListTags(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) ([]domain.DocumentTag, error) {
	args := m.Called(ctx, tenantID, docID, userID, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.DocumentTag), args.Error(1)
}

func (m *MockDocumentService) AddTags(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole, tags map[string]string) ([]domain.DocumentTag, error) {
	args := m.Called(ctx, tenantID, docID, userID, role, tags)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.DocumentTag), args.Error(1)
}

func (m *MockDocumentService) DeleteTag(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole, tagID uuid.UUID) error {
	args := m.Called(ctx, tenantID, docID, userID, role, tagID)
	return args.Error(0)
}

func (m *MockDocumentService) ParseDocument(ctx context.Context, doc *domain.Document, maxAttempts int) {
	m.Called(ctx, doc, maxAttempts)
}

func (m *MockDocumentService) SearchByTag(ctx context.Context, tenantID uuid.UUID, key, value string, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, key, value, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentService) ListSyncReviewQueue(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, filters *port.SyncReviewFilters, offset, limit int) ([]domain.Document, int, error) {
	args := m.Called(ctx, tenantID, userID, role, filters, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Document), args.Int(1), args.Error(2)
}

func (m *MockDocumentService) ApproveSyncBatch(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docIDs []uuid.UUID) error {
	args := m.Called(ctx, tenantID, userID, role, docIDs)
	return args.Error(0)
}

func (m *MockDocumentService) RejectSyncBatch(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docIDs []uuid.UUID, reason string) error {
	args := m.Called(ctx, tenantID, userID, role, docIDs, reason)
	return args.Error(0)
}

func (m *MockDocumentService) ApproveCollectionSync(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, collectionID uuid.UUID) error {
	args := m.Called(ctx, tenantID, userID, role, collectionID)
	return args.Error(0)
}

func (m *MockDocumentService) UpdateVoucherOverrides(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, docID uuid.UUID, overrides *domain.VoucherOverrides) error {
	args := m.Called(ctx, tenantID, userID, role, docID, overrides)
	return args.Error(0)
}
