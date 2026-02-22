package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/parser"
	"satvos/internal/port"
	"satvos/internal/service"
	"satvos/mocks"
)

func setupDocumentService() ( //nolint:gocritic // test helper benefits from multiple named returns
	service.DocumentService,
	*mocks.MockDocumentRepo,
	*mocks.MockFileMetaRepo,
	*mocks.MockCollectionPermissionRepo,
	*mocks.MockDocumentParser,
	*mocks.MockObjectStorage,
	*mocks.MockDocumentTagRepo,
	*mocks.MockUserRepo,
	*mocks.MockDocumentAuditRepo,
) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:   docRepo,
		FileRepo:  fileRepo,
		UserRepo:  userRepo,
		PermRepo:  permRepo,
		TagRepo:   tagRepo,
		AuditRepo: auditRepo,
		Parser:    p,
		Storage:   storage,
	})
	return svc, docRepo, fileRepo, permRepo, p, storage, tagRepo, userRepo, auditRepo
}

// --- CreateAndParse ---

func TestDocumentService_CreateAndParse_Success(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, p, storage, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "tenants/test/files/test.pdf",
		ContentType: "application/pdf",
		Status:      domain.FileStatusUploaded,
	}

	// Admin bypasses collection permission check — permRepo returns error but admin has implicit owner
	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	// Background goroutine calls - we need to allow these
	docRepo.On("GetByID", mock.Anything, mock.Anything, mock.Anything).Return(&domain.Document{
		ID:               uuid.New(),
		TenantID:         tenantID,
		FileID:           fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusPending,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}, nil).Maybe()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	storage.On("Download", mock.Anything, "test-bucket", "tenants/test/files/test.pdf").
		Return([]byte("%PDF-1.4 test content"), nil).Maybe()
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData:   json.RawMessage(`{"invoice":{}}`),
		ConfidenceScores: json.RawMessage(`{"invoice":{}}`),
		ModelUsed:        "test-model",
		PromptUsed:       "test prompt",
	}, nil).Maybe()

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    userID,
		Role:         domain.RoleAdmin,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, collectionID, result.CollectionID)
	assert.Equal(t, fileID, result.FileID)
	assert.Equal(t, "invoice", result.DocumentType)
	assert.Equal(t, domain.ParsingStatusPending, result.ParsingStatus)
	assert.Equal(t, domain.ReviewStatusPending, result.ReviewStatus)
	assert.Equal(t, userID, result.CreatedBy)
	assert.Equal(t, 0, result.ParseAttempts)

	// Wait briefly for goroutine to start (not for completion)
	time.Sleep(50 * time.Millisecond)

	fileRepo.AssertExpectations(t)
}

func TestDocumentService_CreateAndParse_FileNotFound(t *testing.T) {
	svc, _, fileRepo, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	fileID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(nil, domain.ErrNotFound)

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "looking up file")
}

func TestDocumentService_CreateAndParse_DuplicateDocument(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	fileID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:       fileID,
		TenantID: tenantID,
		S3Bucket: "test-bucket",
		S3Key:    "test-key",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Return(domain.ErrDocumentAlreadyExists)

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "creating document")
}

func TestDocumentService_CreateAndParse_CreateRepoError(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	fileID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:       fileID,
		TenantID: tenantID,
		S3Bucket: "test-bucket",
		S3Key:    "test-key",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Return(errors.New("db connection error"))

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.Nil(t, result)
	assert.Error(t, err)
}

// --- GetByID ---

func TestDocumentService_GetByID_Success(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	expected := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(expected, nil)

	result, err := svc.GetByID(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestDocumentService_GetByID_NotFound(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(nil, domain.ErrDocumentNotFound)

	result, err := svc.GetByID(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

// --- GetByFileID ---

func TestDocumentService_GetByFileID_Success(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()

	expected := &domain.Document{
		ID:       uuid.New(),
		TenantID: tenantID,
		FileID:   fileID,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByFileID", mock.Anything, tenantID, fileID).Return(expected, nil)

	result, err := svc.GetByFileID(context.Background(), tenantID, fileID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestDocumentService_GetByFileID_NotFound(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByFileID", mock.Anything, tenantID, fileID).Return(nil, domain.ErrDocumentNotFound)

	result, err := svc.GetByFileID(context.Background(), tenantID, fileID, userID, domain.RoleAdmin)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

// --- ListByCollection ---

func TestDocumentService_ListByCollection_Success(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	collectionID := uuid.New()
	userID := uuid.New()

	expected := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID, CollectionID: collectionID},
		{ID: uuid.New(), TenantID: tenantID, CollectionID: collectionID},
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	filters := &port.DocumentListFilters{}
	docRepo.On("ListByCollection", mock.Anything, tenantID, collectionID, filters, 0, 20).
		Return(expected, 2, nil)

	docs, total, err := svc.ListByCollection(context.Background(), tenantID, collectionID, userID, domain.RoleAdmin, filters, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, docs, 2)
	assert.Equal(t, 2, total)
}

func TestDocumentService_ListByCollection_Empty(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	collectionID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	filters := &port.DocumentListFilters{}
	docRepo.On("ListByCollection", mock.Anything, tenantID, collectionID, filters, 0, 20).
		Return([]domain.Document{}, 0, nil)

	docs, total, err := svc.ListByCollection(context.Background(), tenantID, collectionID, userID, domain.RoleAdmin, filters, 0, 20)

	assert.NoError(t, err)
	assert.Empty(t, docs)
	assert.Equal(t, 0, total)
}

// --- ListByTenant ---

func TestDocumentService_ListByTenant_Success(t *testing.T) {
	svc, docRepo, _, _, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID},
	}

	filters := &port.DocumentListFilters{}
	docRepo.On("ListByTenant", mock.Anything, tenantID, filters, 0, 20).
		Return(expected, 1, nil)

	docs, total, err := svc.ListByTenant(context.Background(), tenantID, userID, domain.RoleAdmin, filters, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, 1, total)
}

// --- UpdateReview ---

func TestDocumentService_UpdateReview_Approved(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	reviewerID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
		ReviewStatus:  domain.ReviewStatusPending,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: reviewerID,
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusApproved,
		Notes:      "Looks good",
	})

	assert.NoError(t, err)
	assert.Equal(t, domain.ReviewStatusApproved, result.ReviewStatus)
	assert.Equal(t, &reviewerID, result.ReviewedBy)
	assert.NotNil(t, result.ReviewedAt)
	assert.Equal(t, "Looks good", result.ReviewerNotes)
}

func TestDocumentService_UpdateReview_Rejected(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	reviewerID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
		ReviewStatus:  domain.ReviewStatusPending,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: reviewerID,
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusRejected,
		Notes:      "Incorrect amounts",
	})

	assert.NoError(t, err)
	assert.Equal(t, domain.ReviewStatusRejected, result.ReviewStatus)
	assert.Equal(t, "Incorrect amounts", result.ReviewerNotes)
}

func TestDocumentService_UpdateReview_NotParsedYet(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusProcessing,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: uuid.New(),
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusApproved,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotParsed)
}

func TestDocumentService_UpdateReview_PendingStatus(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusPending,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: uuid.New(),
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusApproved,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotParsed)
}

func TestDocumentService_UpdateReview_FailedStatus(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusFailed,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: uuid.New(),
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusApproved,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotParsed)
}

func TestDocumentService_UpdateReview_DocNotFound(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(nil, domain.ErrDocumentNotFound)

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: uuid.New(),
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusApproved,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

func TestDocumentService_UpdateReview_RepoError(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Return(errors.New("db error"))

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: uuid.New(),
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusApproved,
	})

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "updating review status")
}

// --- RetryParse ---

func TestDocumentService_RetryParse_Success(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, p, storage, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		FileID:           fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusFailed,
		ParsingError:     "previous error",
		ParseAttempts:    2,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "test-key",
		ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil)
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	storage.On("Download", mock.Anything, "test-bucket", "test-key").
		Return([]byte("%PDF-1.4 test content"), nil).Maybe()
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData:   json.RawMessage(`{"invoice":{}}`),
		ConfidenceScores: json.RawMessage(`{"invoice":{}}`),
		ModelUsed:        "test-model",
		PromptUsed:       "test prompt",
	}, nil).Maybe()

	result, err := svc.RetryParse(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.ParsingStatusPending, result.ParsingStatus)
	assert.Empty(t, result.ParsingError)
	assert.Equal(t, 2, result.ParseAttempts) // Not reset during retry

	// Wait briefly for goroutine to start
	time.Sleep(50 * time.Millisecond)
}

func TestDocumentService_RetryParse_DocNotFound(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(nil, domain.ErrDocumentNotFound)

	result, err := svc.RetryParse(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

func TestDocumentService_RetryParse_FileNotFound(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, _, _, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID:       docID,
		TenantID: tenantID,
		FileID:   fileID,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil)
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(nil, domain.ErrNotFound)

	result, err := svc.RetryParse(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "looking up file for retry")
}

// --- Delete ---

func TestDocumentService_Delete_Success(t *testing.T) {
	svc, docRepo, _, _, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	docRepo.On("Delete", mock.Anything, tenantID, docID).Return(nil)

	err := svc.Delete(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	docRepo.AssertExpectations(t)
}

func TestDocumentService_Delete_NotFound(t *testing.T) {
	svc, docRepo, _, _, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	docRepo.On("Delete", mock.Anything, tenantID, docID).Return(domain.ErrDocumentNotFound)

	err := svc.Delete(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

// --- Background parsing integration test ---

func TestDocumentService_BackgroundParsing_Success(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	tagRepo := new(mocks.MockDocumentTagRepo)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	userRepo := new(mocks.MockUserRepo)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:  docRepo,
		FileRepo: fileRepo,
		UserRepo: userRepo,
		PermRepo: permRepo,
		TagRepo:  tagRepo,
		Parser:   p,
		Storage:  storage,
	})

	tenantID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "tenants/test/files/test.pdf",
		ContentType: "application/pdf",
		Status:      domain.FileStatusUploaded,
	}

	parsedData := json.RawMessage(`{"invoice":{"invoice_number":"INV-001"}}`)
	confidenceScores := json.RawMessage(`{"invoice":{"invoice_number":0.95}}`)

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	// Background goroutine expectations
	docRepo.On("GetByID", mock.Anything, tenantID, mock.AnythingOfType("uuid.UUID")).
		Return(&domain.Document{
			ID:               uuid.New(),
			TenantID:         tenantID,
			FileID:           fileID,
			DocumentType:     "invoice",
			ParsingStatus:    domain.ParsingStatusPending,
			StructuredData:   json.RawMessage("{}"),
			ConfidenceScores: json.RawMessage("{}"),
		}, nil)

	storage.On("Download", mock.Anything, "test-bucket", "tenants/test/files/test.pdf").
		Return([]byte("%PDF-1.4 test content"), nil)

	p.On("Parse", mock.Anything, mock.MatchedBy(func(input port.ParseInput) bool {
		return input.ContentType == "application/pdf" && input.DocumentType == "invoice"
	})).Return(&port.ParseOutput{
		StructuredData:   parsedData,
		ConfidenceScores: confidenceScores,
		ModelUsed:        "claude-sonnet-4-20250514",
		PromptUsed:       "test prompt",
	}, nil)

	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Wait for background goroutine to complete
	time.Sleep(200 * time.Millisecond)

	storage.AssertExpectations(t)
	p.AssertExpectations(t)
}

func TestDocumentService_BackgroundParsing_DownloadFailure(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	tagRepo := new(mocks.MockDocumentTagRepo)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	userRepo := new(mocks.MockUserRepo)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:  docRepo,
		FileRepo: fileRepo,
		UserRepo: userRepo,
		PermRepo: permRepo,
		TagRepo:  tagRepo,
		Parser:   p,
		Storage:  storage,
	})

	tenantID := uuid.New()
	fileID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "test-key",
		ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	docRepo.On("GetByID", mock.Anything, tenantID, mock.AnythingOfType("uuid.UUID")).
		Return(&domain.Document{
			ID:               uuid.New(),
			TenantID:         tenantID,
			FileID:           fileID,
			DocumentType:     "invoice",
			ParsingStatus:    domain.ParsingStatusPending,
			StructuredData:   json.RawMessage("{}"),
			ConfidenceScores: json.RawMessage("{}"),
		}, nil)

	// Set processing status
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	storage.On("Download", mock.Anything, "test-bucket", "test-key").
		Return(nil, errors.New("s3 connection refused"))

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Wait for background goroutine to fail
	time.Sleep(200 * time.Millisecond)

	storage.AssertExpectations(t)
}

func TestDocumentService_BackgroundParsing_ParserFailure(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	tagRepo := new(mocks.MockDocumentTagRepo)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	userRepo := new(mocks.MockUserRepo)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:  docRepo,
		FileRepo: fileRepo,
		UserRepo: userRepo,
		PermRepo: permRepo,
		TagRepo:  tagRepo,
		Parser:   p,
		Storage:  storage,
	})

	tenantID := uuid.New()
	fileID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "test-key",
		ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	docRepo.On("GetByID", mock.Anything, tenantID, mock.AnythingOfType("uuid.UUID")).
		Return(&domain.Document{
			ID:               uuid.New(),
			TenantID:         tenantID,
			FileID:           fileID,
			DocumentType:     "invoice",
			ParsingStatus:    domain.ParsingStatusPending,
			StructuredData:   json.RawMessage("{}"),
			ConfidenceScores: json.RawMessage("{}"),
		}, nil)

	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	storage.On("Download", mock.Anything, "test-bucket", "test-key").
		Return([]byte("%PDF-1.4 content"), nil)

	p.On("Parse", mock.Anything, mock.Anything).
		Return(nil, errors.New("API rate limit exceeded"))

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Wait for background goroutine to fail
	time.Sleep(200 * time.Millisecond)

	p.AssertExpectations(t)
}

// --- CreateAndParse with name and tags ---

func TestDocumentService_CreateAndParse_WithNameAndTags(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, p, storage, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:           fileID,
		TenantID:     tenantID,
		OriginalName: "invoice.pdf",
		S3Bucket:     "test-bucket",
		S3Key:        "test-key",
		ContentType:  "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.MatchedBy(func(doc *domain.Document) bool {
		return doc.Name == "My Custom Name"
	})).Return(nil)
	tagRepo.On("CreateBatch", mock.Anything, mock.MatchedBy(func(tags []domain.DocumentTag) bool {
		return len(tags) == 1 && tags[0].Source == "user"
	})).Return(nil)
	// Background goroutine expectations
	docRepo.On("GetByID", mock.Anything, mock.Anything, mock.Anything).Return(&domain.Document{
		ID: uuid.New(), TenantID: tenantID, FileID: fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusPending,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}, nil).Maybe()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	storage.On("Download", mock.Anything, mock.Anything, mock.Anything).Return([]byte("test"), nil).Maybe()
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"),
		ModelUsed: "m", PromptUsed: "p",
	}, nil).Maybe()
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		FileID:       fileID,
		DocumentType: "invoice",
		Name:         "My Custom Name",
		Tags:         map[string]string{"vendor": "Acme"},
		CreatedBy:    userID,
		Role:         domain.RoleAdmin,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "My Custom Name", result.Name)

	time.Sleep(50 * time.Millisecond)
	tagRepo.AssertExpectations(t)
}

func TestDocumentService_CreateAndParse_DefaultName(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, p, storage, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	fileID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID:           fileID,
		TenantID:     tenantID,
		OriginalName: "invoice_2025.pdf",
		S3Bucket:     "test-bucket",
		S3Key:        "test-key",
		ContentType:  "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	docRepo.On("Create", mock.Anything, mock.MatchedBy(func(doc *domain.Document) bool {
		return doc.Name == "invoice_2025.pdf"
	})).Return(nil)
	// Background goroutine expectations
	docRepo.On("GetByID", mock.Anything, mock.Anything, mock.Anything).Return(&domain.Document{
		ID: uuid.New(), TenantID: tenantID, FileID: fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusPending,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}, nil).Maybe()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	storage.On("Download", mock.Anything, mock.Anything, mock.Anything).Return([]byte("test"), nil).Maybe()
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"),
		ModelUsed: "m", PromptUsed: "p",
	}, nil).Maybe()
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.NoError(t, err)
	assert.Equal(t, "invoice_2025.pdf", result.Name)

	time.Sleep(50 * time.Millisecond)
}

// --- ListTags ---

func TestDocumentService_ListTags_Success(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	doc := &domain.Document{ID: docID, TenantID: tenantID}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(doc, nil)

	expectedTags := []domain.DocumentTag{
		{ID: uuid.New(), DocumentID: docID, Key: "vendor", Value: "Acme", Source: "user"},
	}
	tagRepo.On("ListByDocument", mock.Anything, docID).Return(expectedTags, nil)

	tags, err := svc.ListTags(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.Len(t, tags, 1)
	assert.Equal(t, "vendor", tags[0].Key)
}

// --- AddTags ---

func TestDocumentService_AddTags_Success(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	doc := &domain.Document{ID: docID, TenantID: tenantID}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(doc, nil)
	tagRepo.On("CreateBatch", mock.Anything, mock.MatchedBy(func(tags []domain.DocumentTag) bool {
		return len(tags) == 2
	})).Return(nil)

	tags, err := svc.AddTags(context.Background(), tenantID, docID, userID, domain.RoleAdmin,
		map[string]string{"vendor": "Acme", "year": "2025"})

	assert.NoError(t, err)
	assert.Len(t, tags, 2)
}

// --- SearchByTag ---

func TestDocumentService_SearchByTag_Success(t *testing.T) {
	svc, _, _, _, _, _, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()

	expected := []domain.Document{{ID: uuid.New(), TenantID: tenantID}}
	tagRepo.On("SearchByTag", mock.Anything, tenantID, "vendor", "Acme", 0, 20).
		Return(expected, 1, nil)

	docs, total, err := svc.SearchByTag(context.Background(), tenantID, "vendor", "Acme", 0, 20)

	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, 1, total)
}

// --- RetryParse deletes auto-tags ---

func TestDocumentService_RetryParse_DeletesAutoTags(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, p, storage, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID: docID, TenantID: tenantID, FileID: fileID,
		DocumentType: "invoice", ParsingStatus: domain.ParsingStatusCompleted,
		StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"),
	}
	fileMeta := &domain.FileMeta{
		ID: fileID, TenantID: tenantID,
		S3Bucket: "b", S3Key: "k", ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil).Once()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	storage.On("Download", mock.Anything, "b", "k").Return([]byte("test"), nil).Maybe()
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"),
		ModelUsed: "m", PromptUsed: "p",
	}, nil).Maybe()

	result, err := svc.RetryParse(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	time.Sleep(50 * time.Millisecond)
	tagRepo.AssertCalled(t, "DeleteByDocumentAndSource", mock.Anything, docID, "auto")
}

// --- EditStructuredData ---

func TestDocumentService_EditStructuredData_Success(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()
	collectionID := uuid.New()

	structuredData := json.RawMessage(`{"invoice":{"invoice_number":"INV-001","invoice_date":"2025-01-15"},"seller":{"name":"Acme"},"buyer":{"name":"Buyer"},"line_items":[],"totals":{"total":1000},"payment":{}}`)

	existing := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		CollectionID:     collectionID,
		ParsingStatus:    domain.ParsingStatusCompleted,
		ReviewStatus:     domain.ReviewStatusApproved,
		ValidationStatus: domain.ValidationStatusValid,
		StructuredData:   json.RawMessage(`{"invoice":{}}`),
		ConfidenceScores: json.RawMessage(`{}`),
	}

	updated := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		CollectionID:     collectionID,
		ParsingStatus:    domain.ParsingStatusCompleted,
		ReviewStatus:     domain.ReviewStatusPending,
		ValidationStatus: domain.ValidationStatusPending,
		StructuredData:   structuredData,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil).Once()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil)
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	// Re-fetch after edit
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(updated, nil).Maybe()

	result, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           domain.RoleAdmin,
		StructuredData: structuredData,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	docRepo.AssertCalled(t, "UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document"))
	docRepo.AssertCalled(t, "UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document"))
}

func TestDocumentService_EditStructuredData_NotFound(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(nil, domain.ErrDocumentNotFound)

	result, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           domain.RoleAdmin,
		StructuredData: json.RawMessage(`{}`),
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

func TestDocumentService_EditStructuredData_NotParsed(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusPending,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)

	result, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           domain.RoleAdmin,
		StructuredData: json.RawMessage(`{}`),
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotParsed)
}

func TestDocumentService_EditStructuredData_PermissionDenied(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, userID).
		Return(nil, errors.New("not found"))

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)

	result, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           domain.RoleViewer,
		StructuredData: json.RawMessage(`{}`),
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
}

func TestDocumentService_EditStructuredData_InvalidJSON(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)

	result, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           domain.RoleAdmin,
		StructuredData: json.RawMessage(`not valid json`),
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidStructuredData)
}

func TestDocumentService_EditStructuredData_ResetsReviewStatus(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()
	reviewerID := uuid.New()
	reviewedAt := time.Now().UTC()

	existing := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		ParsingStatus:    domain.ParsingStatusCompleted,
		ReviewStatus:     domain.ReviewStatusApproved,
		ReviewedBy:       &reviewerID,
		ReviewedAt:       &reviewedAt,
		ReviewerNotes:    "Previously approved",
		StructuredData:   json.RawMessage(`{}`),
		ConfidenceScores: json.RawMessage(`{}`),
	}

	updated := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
		ReviewStatus:  domain.ReviewStatusPending,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil).Once()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.MatchedBy(func(doc *domain.Document) bool {
		return doc.ReviewStatus == domain.ReviewStatusPending &&
			doc.ReviewedBy == nil &&
			doc.ReviewedAt == nil &&
			doc.ReviewerNotes == ""
	})).Return(nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil)
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(updated, nil).Maybe()

	structuredData := json.RawMessage(`{"invoice":{},"seller":{},"buyer":{},"line_items":[],"totals":{},"payment":{}}`)
	result, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           domain.RoleAdmin,
		StructuredData: structuredData,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.ReviewStatusPending, result.ReviewStatus)
}

func TestDocumentService_EditStructuredData_ReextractsAutoTags(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		ParsingStatus:    domain.ParsingStatusCompleted,
		StructuredData:   json.RawMessage(`{}`),
		ConfidenceScores: json.RawMessage(`{}`),
	}

	updated := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil).Once()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil).Once()
	tagRepo.On("CreateBatch", mock.Anything, mock.MatchedBy(func(tags []domain.DocumentTag) bool {
		// Should have auto-tags extracted from the invoice data
		for i := range tags {
			if tags[i].Source != "auto" {
				return false
			}
		}
		return true
	})).Return(nil).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(updated, nil).Maybe()

	structuredData := json.RawMessage(`{"invoice":{"invoice_number":"INV-999","invoice_date":"2025-06-01"},"seller":{"name":"Seller Corp","gstin":"29AABCU9603R1ZM"},"buyer":{"name":"Buyer Inc"},"line_items":[],"totals":{"total":5000},"payment":{}}`)

	result, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID:       tenantID,
		DocumentID:     docID,
		UserID:         userID,
		Role:           domain.RoleAdmin,
		StructuredData: structuredData,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	tagRepo.AssertCalled(t, "DeleteByDocumentAndSource", mock.Anything, docID, "auto")
	tagRepo.AssertCalled(t, "CreateBatch", mock.Anything, mock.Anything)
}

// --- Rate limit → queued tests ---

func TestDocumentService_BackgroundParsing_RateLimitQueuesDocument(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	tagRepo := new(mocks.MockDocumentTagRepo)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	userRepo := new(mocks.MockUserRepo)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:  docRepo,
		FileRepo: fileRepo,
		UserRepo: userRepo,
		PermRepo: permRepo,
		TagRepo:  tagRepo,
		Parser:   p,
		Storage:  storage,
	})

	tenantID := uuid.New()
	fileID := uuid.New()
	docID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID: fileID, TenantID: tenantID,
		S3Bucket: "test-bucket", S3Key: "test-key", ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil).Maybe()
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	// Background: GetByID returns doc with 0 attempts
	docRepo.On("GetByID", mock.Anything, tenantID, mock.AnythingOfType("uuid.UUID")).
		Return(&domain.Document{
			ID: docID, TenantID: tenantID, FileID: fileID,
			DocumentType:     "invoice",
			ParsingStatus:    domain.ParsingStatusPending,
			StructuredData:   json.RawMessage("{}"),
			ConfidenceScores: json.RawMessage("{}"),
		}, nil).Maybe()

	storage.On("Download", mock.Anything, "test-bucket", "test-key").
		Return([]byte("%PDF-1.4 content"), nil).Maybe()

	// Parser returns RateLimitError
	rlErr := parser.NewRateLimitError("claude", fmt.Errorf("429 Too Many Requests"), 30)
	p.On("Parse", mock.Anything, mock.Anything).Return(nil, rlErr).Maybe()

	// Capture the final doc state via Run callback (thread-safe channel)
	finalStatus := make(chan domain.ParsingStatus, 2)
	finalRetryAfter := make(chan bool, 2) // true if non-nil
	finalError := make(chan string, 2)
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			doc := args.Get(1).(*domain.Document)
			finalStatus <- doc.ParsingStatus
			finalRetryAfter <- (doc.RetryAfter != nil)
			finalError <- doc.ParsingError
		}).Return(nil).Maybe()

	_, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})
	assert.NoError(t, err)

	// Wait for background goroutine — two UpdateStructuredData calls expected
	// 1st: set processing, 2nd: set queued
	var lastStatus domain.ParsingStatus
	var lastHasRetry bool
	var lastErrMsg string
	for i := 0; i < 2; i++ {
		select {
		case lastStatus = <-finalStatus:
			lastHasRetry = <-finalRetryAfter
			lastErrMsg = <-finalError
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for UpdateStructuredData call")
		}
	}
	assert.Equal(t, domain.ParsingStatusQueued, lastStatus)
	assert.True(t, lastHasRetry)
	assert.Contains(t, lastErrMsg, "rate limited")
}

func TestDocumentService_BackgroundParsing_RateLimitExceedsMaxAttempts(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	tagRepo := new(mocks.MockDocumentTagRepo)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	userRepo := new(mocks.MockUserRepo)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:  docRepo,
		FileRepo: fileRepo,
		UserRepo: userRepo,
		PermRepo: permRepo,
		TagRepo:  tagRepo,
		Parser:   p,
		Storage:  storage,
	})

	tenantID := uuid.New()
	fileID := uuid.New()
	docID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID: fileID, TenantID: tenantID,
		S3Bucket: "test-bucket", S3Key: "test-key", ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil).Maybe()
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	// Background: GetByID returns doc with 4 attempts (will become 5 after increment = max)
	docRepo.On("GetByID", mock.Anything, tenantID, mock.AnythingOfType("uuid.UUID")).
		Return(&domain.Document{
			ID: docID, TenantID: tenantID, FileID: fileID,
			DocumentType:     "invoice",
			ParseAttempts:    4,
			ParsingStatus:    domain.ParsingStatusPending,
			StructuredData:   json.RawMessage("{}"),
			ConfidenceScores: json.RawMessage("{}"),
		}, nil).Maybe()

	storage.On("Download", mock.Anything, "test-bucket", "test-key").
		Return([]byte("%PDF-1.4 content"), nil).Maybe()

	rlErr := parser.NewRateLimitError("claude", fmt.Errorf("429 Too Many Requests"), 30)
	p.On("Parse", mock.Anything, mock.Anything).Return(nil, rlErr).Maybe()

	finalStatus := make(chan domain.ParsingStatus, 2)
	finalRetryAfter := make(chan bool, 2)
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			doc := args.Get(1).(*domain.Document)
			finalStatus <- doc.ParsingStatus
			finalRetryAfter <- (doc.RetryAfter != nil)
		}).Return(nil).Maybe()

	_, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})
	assert.NoError(t, err)

	// Wait for two UpdateStructuredData calls: 1) processing, 2) failed
	var lastStatus domain.ParsingStatus
	var lastHasRetry bool
	for i := 0; i < 2; i++ {
		select {
		case lastStatus = <-finalStatus:
			lastHasRetry = <-finalRetryAfter
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for UpdateStructuredData call")
		}
	}
	assert.Equal(t, domain.ParsingStatusFailed, lastStatus)
	assert.False(t, lastHasRetry)
}

func TestDocumentService_BackgroundParsing_NonRateLimitErrorStillFails(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	tagRepo := new(mocks.MockDocumentTagRepo)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	userRepo := new(mocks.MockUserRepo)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:  docRepo,
		FileRepo: fileRepo,
		UserRepo: userRepo,
		PermRepo: permRepo,
		TagRepo:  tagRepo,
		Parser:   p,
		Storage:  storage,
	})

	tenantID := uuid.New()
	fileID := uuid.New()
	docID := uuid.New()

	fileMeta := &domain.FileMeta{
		ID: fileID, TenantID: tenantID,
		S3Bucket: "test-bucket", S3Key: "test-key", ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil).Maybe()
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	docRepo.On("GetByID", mock.Anything, tenantID, mock.AnythingOfType("uuid.UUID")).
		Return(&domain.Document{
			ID: docID, TenantID: tenantID, FileID: fileID,
			DocumentType:     "invoice",
			ParsingStatus:    domain.ParsingStatusPending,
			StructuredData:   json.RawMessage("{}"),
			ConfidenceScores: json.RawMessage("{}"),
		}, nil).Maybe()

	storage.On("Download", mock.Anything, "test-bucket", "test-key").
		Return([]byte("%PDF-1.4 content"), nil).Maybe()

	// Regular error (not RateLimitError)
	p.On("Parse", mock.Anything, mock.Anything).Return(nil, errors.New("invalid API key")).Maybe()

	finalStatus := make(chan domain.ParsingStatus, 2)
	finalRetryAfter := make(chan bool, 2)
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).
		Run(func(args mock.Arguments) {
			doc := args.Get(1).(*domain.Document)
			finalStatus <- doc.ParsingStatus
			finalRetryAfter <- (doc.RetryAfter != nil)
		}).Return(nil).Maybe()

	_, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: uuid.New(),
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})
	assert.NoError(t, err)

	// Wait for two UpdateStructuredData calls: 1) processing, 2) failed
	var lastStatus domain.ParsingStatus
	var lastHasRetry bool
	for i := 0; i < 2; i++ {
		select {
		case lastStatus = <-finalStatus:
			lastHasRetry = <-finalRetryAfter
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for UpdateStructuredData call")
		}
	}
	assert.Equal(t, domain.ParsingStatusFailed, lastStatus)
	assert.False(t, lastHasRetry)
}

// --- Audit Trail ---

func TestDocumentService_CreateAndParse_WritesAuditEntry(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, p, storage, _, _, auditRepo := setupDocumentService()

	tenantID := uuid.New()
	userID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(&domain.FileMeta{
		ID: fileID, TenantID: tenantID, S3Bucket: "b", S3Key: "k", ContentType: "application/pdf",
	}, nil)
	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	docRepo.On("GetByID", mock.Anything, mock.Anything, mock.Anything).Return(&domain.Document{
		ID: uuid.New(), TenantID: tenantID, FileID: fileID, DocumentType: "invoice",
		ParsingStatus: domain.ParsingStatusPending, StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"),
	}, nil).Maybe()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	storage.On("Download", mock.Anything, mock.Anything, mock.Anything).Return([]byte("test"), nil).Maybe()
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"), ModelUsed: "m", PromptUsed: "p",
	}, nil).Maybe()

	_, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID: tenantID, CollectionID: collectionID, FileID: fileID,
		DocumentType: "invoice", CreatedBy: userID, Role: domain.RoleAdmin,
	})
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Verify audit was called with document.created action
	auditRepo.AssertCalled(t, "Create", mock.Anything, mock.MatchedBy(func(entry *domain.DocumentAuditEntry) bool {
		return entry.Action == string(domain.AuditDocumentCreated) && entry.TenantID == tenantID && *entry.UserID == userID
	}))
}

func TestDocumentService_UpdateReview_WritesAuditEntry(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, auditRepo := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	reviewerID := uuid.New()

	existing := &domain.Document{
		ID: docID, TenantID: tenantID,
		ParsingStatus: domain.ParsingStatusCompleted, ReviewStatus: domain.ReviewStatusPending,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	_, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID: tenantID, DocumentID: docID, ReviewerID: reviewerID,
		Role: domain.RoleAdmin, Status: domain.ReviewStatusApproved, Notes: "LGTM",
	})
	assert.NoError(t, err)

	auditRepo.AssertCalled(t, "Create", mock.Anything, mock.MatchedBy(func(entry *domain.DocumentAuditEntry) bool {
		return entry.Action == string(domain.AuditDocumentReview) && *entry.UserID == reviewerID
	}))
}

func TestDocumentService_EditStructuredData_WritesAuditEntry(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, tagRepo, _, auditRepo := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	userID := uuid.New()

	existing := &domain.Document{
		ID: docID, TenantID: tenantID,
		ParsingStatus: domain.ParsingStatusCompleted, StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"),
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil).Once()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil)
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil).Maybe()

	structuredData := json.RawMessage(`{"invoice":{},"seller":{},"buyer":{},"line_items":[],"totals":{},"payment":{}}`)
	_, err := svc.EditStructuredData(context.Background(), &service.EditStructuredDataInput{
		TenantID: tenantID, DocumentID: docID, UserID: userID,
		Role: domain.RoleAdmin, StructuredData: structuredData,
	})
	assert.NoError(t, err)

	auditRepo.AssertCalled(t, "Create", mock.Anything, mock.MatchedBy(func(entry *domain.DocumentAuditEntry) bool {
		return entry.Action == string(domain.AuditDocumentEditStructured) && *entry.UserID == userID
	}))
}

func TestDocumentService_AuditFailureDoesNotBlockOperation(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Audit repo always fails
	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(errors.New("db down")).Maybe()

	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:   docRepo,
		FileRepo:  fileRepo,
		UserRepo:  userRepo,
		PermRepo:  permRepo,
		TagRepo:   tagRepo,
		AuditRepo: auditRepo,
		Parser:    p,
		Storage:   storage,
	})

	tenantID := uuid.New()
	docID := uuid.New()

	docRepo.On("Delete", mock.Anything, tenantID, docID).Return(nil)

	err := svc.Delete(context.Background(), tenantID, docID, uuid.New(), domain.RoleAdmin)
	assert.NoError(t, err) // audit failure must not block deletion
}

// --- AssignDocument ---

func TestDocumentService_AssignDocument_Success(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, userRepo, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	callerID := uuid.New()
	assigneeID := uuid.New()
	collectionID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		CollectionID:  collectionID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	// Caller has editor perm (admin implicit)
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, callerID).
		Return(nil, errors.New("not found"))
	// Assignee has editor perm
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, assigneeID).
		Return(editorPerm(collectionID, assigneeID), nil)

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	userRepo.On("GetByID", mock.Anything, tenantID, assigneeID).Return(&domain.User{
		ID:       assigneeID,
		TenantID: tenantID,
		Role:     domain.RoleMember,
	}, nil)
	docRepo.On("UpdateAssignment", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	result, err := svc.AssignDocument(context.Background(), &service.AssignDocumentInput{
		TenantID:   tenantID,
		DocumentID: docID,
		CallerID:   callerID,
		CallerRole: domain.RoleAdmin,
		AssigneeID: &assigneeID,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, &assigneeID, result.AssignedTo)
	assert.NotNil(t, result.AssignedAt)
	assert.Equal(t, &callerID, result.AssignedBy)
	docRepo.AssertExpectations(t)
}

func TestDocumentService_AssignDocument_Unassign(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	callerID := uuid.New()
	assigneeID := uuid.New()
	collectionID := uuid.New()
	assignedAt := time.Now().UTC()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		CollectionID:  collectionID,
		ParsingStatus: domain.ParsingStatusCompleted,
		AssignedTo:    &assigneeID,
		AssignedAt:    &assignedAt,
		AssignedBy:    &callerID,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, callerID).
		Return(nil, errors.New("not found"))

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	docRepo.On("UpdateAssignment", mock.Anything, mock.MatchedBy(func(doc *domain.Document) bool {
		return doc.AssignedTo == nil && doc.AssignedAt == nil && doc.AssignedBy == nil
	})).Return(nil)

	result, err := svc.AssignDocument(context.Background(), &service.AssignDocumentInput{
		TenantID:   tenantID,
		DocumentID: docID,
		CallerID:   callerID,
		CallerRole: domain.RoleAdmin,
		AssigneeID: nil, // unassign
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Nil(t, result.AssignedTo)
	assert.Nil(t, result.AssignedAt)
	assert.Nil(t, result.AssignedBy)
}

func TestDocumentService_AssignDocument_NotParsed(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	callerID := uuid.New()
	assigneeID := uuid.New()
	collectionID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		CollectionID:  collectionID,
		ParsingStatus: domain.ParsingStatusProcessing,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, callerID).
		Return(nil, errors.New("not found"))

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)

	result, err := svc.AssignDocument(context.Background(), &service.AssignDocumentInput{
		TenantID:   tenantID,
		DocumentID: docID,
		CallerID:   callerID,
		CallerRole: domain.RoleAdmin,
		AssigneeID: &assigneeID,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotParsed)
}

func TestDocumentService_AssignDocument_AssigneeCannotReview(t *testing.T) {
	svc, docRepo, _, permRepo, _, _, _, userRepo, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	callerID := uuid.New()
	assigneeID := uuid.New()
	collectionID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		CollectionID:  collectionID,
		ParsingStatus: domain.ParsingStatusCompleted,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, callerID).
		Return(nil, errors.New("not found"))
	// Assignee has no explicit perm and is a viewer → no access
	permRepo.On("GetByCollectionAndUser", mock.Anything, collectionID, assigneeID).
		Return(nil, errors.New("not found"))

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	userRepo.On("GetByID", mock.Anything, tenantID, assigneeID).Return(&domain.User{
		ID:       assigneeID,
		TenantID: tenantID,
		Role:     domain.RoleViewer,
	}, nil)

	result, err := svc.AssignDocument(context.Background(), &service.AssignDocumentInput{
		TenantID:   tenantID,
		DocumentID: docID,
		CallerID:   callerID,
		CallerRole: domain.RoleAdmin,
		AssigneeID: &assigneeID,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrAssigneeCannotReview)
}

func TestDocumentService_AssignDocument_DocNotFound(t *testing.T) {
	svc, docRepo, _, _, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	assigneeID := uuid.New()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(nil, domain.ErrDocumentNotFound)

	result, err := svc.AssignDocument(context.Background(), &service.AssignDocumentInput{
		TenantID:   tenantID,
		DocumentID: docID,
		CallerID:   uuid.New(),
		CallerRole: domain.RoleAdmin,
		AssigneeID: &assigneeID,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrDocumentNotFound)
}

// --- ListReviewQueue ---

func TestDocumentService_ListReviewQueue_Success(t *testing.T) {
	svc, docRepo, _, _, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.Document{
		{ID: uuid.New(), TenantID: tenantID, AssignedTo: &userID, ParsingStatus: domain.ParsingStatusCompleted},
	}

	docRepo.On("ListReviewQueue", mock.Anything, tenantID, userID, 0, 20).Return(expected, 1, nil)

	docs, total, err := svc.ListReviewQueue(context.Background(), tenantID, userID, 0, 20)

	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, 1, total)
}

func TestDocumentService_ListReviewQueue_Empty(t *testing.T) {
	svc, docRepo, _, _, _, _, _, _, _ := setupDocumentService()

	tenantID := uuid.New()
	userID := uuid.New()

	docRepo.On("ListReviewQueue", mock.Anything, tenantID, userID, 0, 20).Return([]domain.Document{}, 0, nil)

	docs, total, err := svc.ListReviewQueue(context.Background(), tenantID, userID, 0, 20)

	assert.NoError(t, err)
	assert.Empty(t, docs)
	assert.Equal(t, 0, total)
}

// --- RetryParse clears assignment ---

func TestDocumentService_RetryParse_ClearsAssignment(t *testing.T) {
	svc, docRepo, fileRepo, permRepo, p, storage, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()
	userID := uuid.New()
	assigneeID := uuid.New()
	assignedAt := time.Now().UTC()

	existing := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		FileID:           fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusFailed,
		AssignedTo:       &assigneeID,
		AssignedAt:       &assignedAt,
		AssignedBy:       &userID,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}

	fileMeta := &domain.FileMeta{
		ID: fileID, TenantID: tenantID,
		S3Bucket: "b", S3Key: "k", ContentType: "application/pdf",
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil)
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	storage.On("Download", mock.Anything, "b", "k").Return([]byte("test"), nil).Maybe()
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData: json.RawMessage("{}"), ConfidenceScores: json.RawMessage("{}"),
		ModelUsed: "m", PromptUsed: "p",
	}, nil).Maybe()

	result, err := svc.RetryParse(context.Background(), tenantID, docID, userID, domain.RoleAdmin)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Assignment fields should be cleared
	assert.Nil(t, result.AssignedTo)
	assert.Nil(t, result.AssignedAt)
	assert.Nil(t, result.AssignedBy)

	time.Sleep(50 * time.Millisecond)
}

// --- Summary Upsert Integration ---

func TestDocumentService_ParseDocument_UpsertsSummary(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	summaryRepo := new(mocks.MockDocumentSummaryRepo)

	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(nil).Maybe()

	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:     docRepo,
		FileRepo:    fileRepo,
		UserRepo:    userRepo,
		PermRepo:    permRepo,
		TagRepo:     tagRepo,
		AuditRepo:   auditRepo,
		SummaryRepo: summaryRepo,
		Parser:      p,
		Storage:     storage,
	})

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()
	collectionID := uuid.New()

	structuredJSON := json.RawMessage(`{
		"invoice": {"invoice_number": "INV-001", "invoice_date": "2025-01-15", "invoice_type": "Tax Invoice", "currency": "INR", "place_of_supply": "Karnataka", "reverse_charge": false, "irn": ""},
		"seller": {"name": "Test Seller", "gstin": "29AABCT1332L1ZP", "state": "Karnataka", "state_code": "29", "address": "Bangalore"},
		"buyer": {"name": "Test Buyer", "gstin": "27AABCU9603R1ZM", "state": "Maharashtra", "state_code": "27", "address": "Mumbai"},
		"line_items": [{"description": "Item 1", "hsn_sac_code": "8471", "quantity": 1, "unit_price": 1000, "total_amount": 1000, "taxable_amount": 1000, "cgst_amount": 0, "sgst_amount": 0, "igst_amount": 180, "igst_rate": 18}],
		"totals": {"subtotal": 1000, "total_discount": 0, "taxable_amount": 1000, "cgst": 0, "sgst": 0, "igst": 180, "cess": 0, "total": 1180},
		"payment": {},
		"confidence_scores": {}
	}`)

	doc := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		CollectionID:     collectionID,
		FileID:           fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusProcessing,
		ParseAttempts:    1,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "test-key",
		ContentType: "application/pdf",
	}

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	storage.On("Download", mock.Anything, "test-bucket", "test-key").Return([]byte("%PDF-1.4 test"), nil)
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData:   structuredJSON,
		ConfidenceScores: json.RawMessage(`{}`),
		ModelUsed:        "test-model",
		PromptUsed:       "test prompt",
	}, nil)
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

	summaryRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(s *domain.DocumentSummary) bool {
		return s.TenantID == tenantID &&
			s.DocumentID == docID &&
			s.CollectionID == collectionID &&
			s.SellerGSTIN == "29AABCT1332L1ZP" &&
			s.BuyerGSTIN == "27AABCU9603R1ZM" &&
			s.TotalAmount == 1180 &&
			s.InvoiceNumber == "INV-001" &&
			s.LineItemCount == 1 &&
			s.SellerName == "Test Seller" &&
			s.BuyerName == "Test Buyer" &&
			s.IGST == 180
	})).Return(nil)
	summaryRepo.On("ReplaceLineItems", mock.Anything, docID, tenantID, mock.AnythingOfType("[]domain.DocumentLineItem")).Return(nil)
	summaryRepo.On("UpdateStatuses", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	svc.ParseDocument(context.Background(), doc, 5)

	summaryRepo.AssertExpectations(t)
	docRepo.AssertExpectations(t)
}

func TestDocumentService_UpdateReview_UpdatesSummaryStatuses(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	summaryRepo := new(mocks.MockDocumentSummaryRepo)

	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(nil).Maybe()

	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:     docRepo,
		FileRepo:    fileRepo,
		UserRepo:    userRepo,
		PermRepo:    permRepo,
		TagRepo:     tagRepo,
		AuditRepo:   auditRepo,
		SummaryRepo: summaryRepo,
		Parser:      p,
		Storage:     storage,
	})

	tenantID := uuid.New()
	docID := uuid.New()
	reviewerID := uuid.New()

	existing := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ParsingStatus: domain.ParsingStatusCompleted,
		ReviewStatus:  domain.ReviewStatusPending,
	}

	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	docRepo.On("GetByID", mock.Anything, tenantID, docID).Return(existing, nil)
	docRepo.On("UpdateReviewStatus", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)

	summaryRepo.On("UpdateStatuses", mock.Anything, docID, mock.MatchedBy(func(s domain.SummaryStatusUpdate) bool {
		return s.ReviewStatus == domain.ReviewStatusApproved
	})).Return(nil)
	summaryRepo.On("Upsert", mock.Anything, mock.Anything).Return(nil).Maybe()

	result, err := svc.UpdateReview(context.Background(), &service.UpdateReviewInput{
		TenantID:   tenantID,
		DocumentID: docID,
		ReviewerID: reviewerID,
		Role:       domain.RoleAdmin,
		Status:     domain.ReviewStatusApproved,
		Notes:      "LGTM",
	})

	assert.NoError(t, err)
	assert.Equal(t, domain.ReviewStatusApproved, result.ReviewStatus)
	summaryRepo.AssertExpectations(t)
}

func TestDocumentService_SummaryUpsertFailure_DoesNotFailParse(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)
	summaryRepo := new(mocks.MockDocumentSummaryRepo)

	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(nil).Maybe()

	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:     docRepo,
		FileRepo:    fileRepo,
		UserRepo:    userRepo,
		PermRepo:    permRepo,
		TagRepo:     tagRepo,
		AuditRepo:   auditRepo,
		SummaryRepo: summaryRepo,
		Parser:      p,
		Storage:     storage,
	})

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()

	structuredJSON := json.RawMessage(`{
		"invoice": {"invoice_number": "INV-002"},
		"seller": {"name": "S", "gstin": "29AABCT1332L1ZP"},
		"buyer": {"name": "B", "gstin": "27AABCU9603R1ZM"},
		"line_items": [],
		"totals": {"total": 500},
		"payment": {},
		"confidence_scores": {}
	}`)

	doc := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		FileID:           fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusProcessing,
		ParseAttempts:    1,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "test-key",
		ContentType: "application/pdf",
	}

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	storage.On("Download", mock.Anything, "test-bucket", "test-key").Return([]byte("%PDF-1.4 test"), nil)
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData:   structuredJSON,
		ConfidenceScores: json.RawMessage(`{}`),
		ModelUsed:        "test-model",
		PromptUsed:       "test prompt",
	}, nil)
	docRepo.On("UpdateStructuredData", mock.Anything, mock.MatchedBy(func(d *domain.Document) bool {
		return d.ParsingStatus == domain.ParsingStatusCompleted
	})).Return(nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Summary upsert fails — should NOT cause parse to fail
	summaryRepo.On("Upsert", mock.Anything, mock.Anything).Return(errors.New("db error"))
	summaryRepo.On("ReplaceLineItems", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("db error")).Maybe()
	summaryRepo.On("UpdateStatuses", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("db error")).Maybe()

	// Should not panic
	assert.NotPanics(t, func() {
		svc.ParseDocument(context.Background(), doc, 5)
	})

	// Verify parse result was still saved with completed status
	docRepo.AssertCalled(t, "UpdateStructuredData", mock.Anything, mock.MatchedBy(func(d *domain.Document) bool {
		return d.ParsingStatus == domain.ParsingStatusCompleted
	}))
}

func TestDocumentService_NilSummaryRepo_NoPanic(t *testing.T) {
	// Uses the default setupDocumentService which passes nil for summaryRepo
	svc, docRepo, fileRepo, _, p, storage, tagRepo, _, _ := setupDocumentService()

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()

	structuredJSON := json.RawMessage(`{
		"invoice": {"invoice_number": "INV-003"},
		"seller": {"name": "S"},
		"buyer": {"name": "B"},
		"line_items": [],
		"totals": {"total": 100},
		"payment": {},
		"confidence_scores": {}
	}`)

	doc := &domain.Document{
		ID:               docID,
		TenantID:         tenantID,
		FileID:           fileID,
		DocumentType:     "invoice",
		ParsingStatus:    domain.ParsingStatusProcessing,
		ParseAttempts:    1,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}

	fileMeta := &domain.FileMeta{
		ID:          fileID,
		TenantID:    tenantID,
		S3Bucket:    "test-bucket",
		S3Key:       "test-key",
		ContentType: "application/pdf",
	}

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).Return(fileMeta, nil)
	storage.On("Download", mock.Anything, "test-bucket", "test-key").Return([]byte("%PDF-1.4 test"), nil)
	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{
		StructuredData:   structuredJSON,
		ConfidenceScores: json.RawMessage(`{}`),
		ModelUsed:        "test-model",
		PromptUsed:       "test prompt",
	}, nil)
	docRepo.On("UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil)
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, docID, "auto").Return(nil).Maybe()
	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

	// Should not panic even though summaryRepo is nil
	assert.NotPanics(t, func() {
		svc.ParseDocument(context.Background(), doc, 5)
	})

	docRepo.AssertCalled(t, "UpdateStructuredData", mock.Anything, mock.AnythingOfType("*domain.Document"))
}
