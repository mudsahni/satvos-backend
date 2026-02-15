package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

// --- CreateAndParse error paths ---

func TestDocumentService_CreateAndParse_QuotaExceeded(t *testing.T) {
	// Can't use setupDocumentService() because it pre-registers CheckAndIncrementQuota→nil
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)

	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(nil).Maybe()
	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

	// Quota check returns ErrQuotaExceeded
	userRepo.On("CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything).
		Return(domain.ErrQuotaExceeded)

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

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     uuid.New(),
		CollectionID: uuid.New(),
		FileID:       uuid.New(),
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleAdmin,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrQuotaExceeded)
	docRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestDocumentService_CreateAndParse_PermissionDenied_ViewerRole(t *testing.T) {
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

	// No explicit permission — viewer role has implicit viewer (level 1), below editor (level 2)
	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

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

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     uuid.New(),
		CollectionID: uuid.New(),
		FileID:       uuid.New(),
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleViewer,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
	docRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	fileRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything, mock.Anything)
}

func TestDocumentService_CreateAndParse_PermissionDenied_FreeRole(t *testing.T) {
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

	// No explicit permission — free role has no implicit collection permission
	permRepo.On("GetByCollectionAndUser", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("not found")).Maybe()

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

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     uuid.New(),
		CollectionID: uuid.New(),
		FileID:       uuid.New(),
		DocumentType: "invoice",
		CreatedBy:    uuid.New(),
		Role:         domain.RoleFree,
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCollectionPermDenied)
	docRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}
