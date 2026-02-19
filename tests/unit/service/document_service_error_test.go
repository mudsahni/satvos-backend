package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/port"
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

func TestDocumentService_CreateAndParse_ServiceAccount_SkipsQuota(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	fileRepo := new(mocks.MockFileMetaRepo)
	userRepo := new(mocks.MockUserRepo)
	permRepo := new(mocks.MockCollectionPermissionRepo)
	saPermRepo := new(mocks.MockServiceAccountPermissionRepo)
	tagRepo := new(mocks.MockDocumentTagRepo)
	auditRepo := new(mocks.MockDocumentAuditRepo)
	p := new(mocks.MockDocumentParser)
	storage := new(mocks.MockObjectStorage)

	tenantID := uuid.New()
	collectionID := uuid.New()
	saID := uuid.New()
	fileID := uuid.New()

	// SA has editor permission on the collection
	saPermRepo.On("GetByAccountAndCollection", mock.Anything, saID, collectionID).
		Return(&domain.ServiceAccountPermission{Permission: domain.CollectionPermEditor}, nil).Maybe()

	auditRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.DocumentAuditEntry")).Return(nil).Maybe()

	fileRepo.On("GetByID", mock.Anything, tenantID, fileID).
		Return(&domain.FileMeta{ID: fileID, OriginalName: "invoice.pdf", ContentType: "application/pdf", S3Bucket: "b", S3Key: "k"}, nil).Maybe()
	// Background goroutine re-fetches with the doc's zero-value fields; allow any args
	fileRepo.On("GetByID", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.FileMeta{S3Bucket: "b", S3Key: "k", ContentType: "application/pdf"}, nil).Maybe()

	docRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Document")).Return(nil).Maybe()
	docRepo.On("GetByID", mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.Document{TenantID: tenantID, FileID: fileID, ParseMode: domain.ParseModeSingle}, nil).Maybe()
	docRepo.On("UpdateStructuredData", mock.Anything, mock.Anything).Return(nil).Maybe()

	tagRepo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil).Maybe()
	tagRepo.On("DeleteByDocumentAndSource", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	p.On("Parse", mock.Anything, mock.Anything).Return(&port.ParseOutput{}, nil).Maybe()
	storage.On("Download", mock.Anything, mock.Anything, mock.Anything).Return([]byte("pdf"), nil).Maybe()

	svc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:    docRepo,
		FileRepo:   fileRepo,
		UserRepo:   userRepo,
		PermRepo:   permRepo,
		SAPermRepo: saPermRepo,
		TagRepo:    tagRepo,
		AuditRepo:  auditRepo,
		Parser:     p,
		Storage:    storage,
	})

	result, err := svc.CreateAndParse(context.Background(), &service.CreateDocumentInput{
		TenantID:     tenantID,
		CollectionID: collectionID,
		FileID:       fileID,
		DocumentType: "invoice",
		CreatedBy:    saID,
		Role:         domain.RoleService,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// CheckAndIncrementQuota should never be called for service accounts
	userRepo.AssertNotCalled(t, "CheckAndIncrementQuota", mock.Anything, mock.Anything, mock.Anything)
}
