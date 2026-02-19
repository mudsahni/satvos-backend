package service

import (
	"context"
	"fmt"
	"log"
	"mime/multipart"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// CreateCollectionInput is the DTO for creating a collection.
type CreateCollectionInput struct {
	TenantID    uuid.UUID
	CreatedBy   uuid.UUID
	Role        domain.UserRole
	Name        string
	Description string
	OwnerEmail  string // optional: if set, look up user by email and grant owner permission
}

// UpdateCollectionInput is the DTO for updating a collection.
type UpdateCollectionInput struct {
	TenantID     uuid.UUID
	CollectionID uuid.UUID
	UserID       uuid.UUID
	Role         domain.UserRole
	Name         string
	Description  string
}

// SetPermissionInput is the DTO for setting a collection permission.
type SetPermissionInput struct {
	TenantID     uuid.UUID
	CollectionID uuid.UUID
	GrantedBy    uuid.UUID
	CallerRole   domain.UserRole
	UserID       uuid.UUID
	Permission   domain.CollectionPermission
}

// BatchUploadFileInput represents a single file in a batch upload.
type BatchUploadFileInput struct {
	File   multipart.File
	Header *multipart.FileHeader
}

// BatchUploadResult contains per-file results from a batch upload.
type BatchUploadResult struct {
	FileName string           `json:"file_name"`
	Success  bool             `json:"success"`
	File     *domain.FileMeta `json:"file,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// CollectionService defines the collection management contract.
type CollectionService interface {
	Create(ctx context.Context, input *CreateCollectionInput) (*domain.Collection, error)
	GetByID(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole) (*domain.Collection, error)
	List(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, offset, limit int) ([]domain.Collection, int, error)
	Update(ctx context.Context, input *UpdateCollectionInput) (*domain.Collection, error)
	Delete(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole) error
	ListFiles(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, offset, limit int) ([]domain.FileMeta, int, error)
	BatchUploadFiles(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, files []BatchUploadFileInput) ([]BatchUploadResult, error)
	RemoveFile(ctx context.Context, tenantID, collectionID, fileID, userID uuid.UUID, role domain.UserRole) error
	AddFileToCollection(ctx context.Context, tenantID, collectionID, fileID, userID uuid.UUID, role domain.UserRole) error
	SetPermission(ctx context.Context, input *SetPermissionInput) error
	ListPermissions(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, offset, limit int) ([]domain.CollectionPermissionEntry, int, error)
	RemovePermission(ctx context.Context, tenantID, collectionID, targetUserID, userID uuid.UUID, role domain.UserRole) error
	EffectivePermission(ctx context.Context, collectionID, userID uuid.UUID, role domain.UserRole) domain.CollectionPermission
	EffectivePermissions(ctx context.Context, collectionIDs []uuid.UUID, userID uuid.UUID, role domain.UserRole) (map[uuid.UUID]domain.CollectionPermission, error)
}

type collectionService struct {
	collectionRepo port.CollectionRepository
	permRepo       port.CollectionPermissionRepository
	fileRepo       port.CollectionFileRepository
	fileSvc        FileService
	userRepo       port.UserRepository
	saPermRepo     port.ServiceAccountPermissionRepository // optional, nil-safe
}

// NewCollectionService creates a new CollectionService implementation.
func NewCollectionService(
	collectionRepo port.CollectionRepository,
	permRepo port.CollectionPermissionRepository,
	fileRepo port.CollectionFileRepository,
	fileSvc FileService,
	userRepo port.UserRepository,
	saPermRepo port.ServiceAccountPermissionRepository,
) CollectionService {
	return &collectionService{
		collectionRepo: collectionRepo,
		permRepo:       permRepo,
		fileRepo:       fileRepo,
		fileSvc:        fileSvc,
		userRepo:       userRepo,
		saPermRepo:     saPermRepo,
	}
}

// effectivePermission delegates to the shared permission helpers, branching for service accounts.
func (s *collectionService) effectivePermission(ctx context.Context, collectionID, userID uuid.UUID, role domain.UserRole) domain.CollectionPermission {
	if role == domain.RoleService {
		if s.saPermRepo == nil {
			return ""
		}
		return ServiceAccountCollectionPerm(ctx, s.saPermRepo, collectionID, userID)
	}
	return EffectiveCollectionPerm(ctx, s.permRepo, collectionID, userID, role)
}

// requirePermission delegates to the shared permission helpers, branching for service accounts.
func (s *collectionService) requirePermission(ctx context.Context, collectionID, userID uuid.UUID, role domain.UserRole, minLevel domain.CollectionPermission) error {
	if role == domain.RoleService {
		if s.saPermRepo == nil {
			return domain.ErrCollectionPermDenied
		}
		return RequireServiceAccountCollectionPerm(ctx, s.saPermRepo, collectionID, userID, minLevel)
	}
	return RequireCollectionPerm(ctx, s.permRepo, collectionID, userID, role, minLevel)
}

func (s *collectionService) Create(ctx context.Context, input *CreateCollectionInput) (*domain.Collection, error) {
	// Viewers cannot create collections.
	// Note: RoleFree is blocked at the router level (not listed in RequireRole).
	if input.Role == domain.RoleViewer {
		return nil, domain.ErrInsufficientRole
	}

	// Service accounts need saPermRepo to be configured
	if input.Role == domain.RoleService && s.saPermRepo == nil {
		return nil, domain.ErrInsufficientRole
	}

	collection := &domain.Collection{
		ID:          uuid.New(),
		TenantID:    input.TenantID,
		Name:        input.Name,
		Description: input.Description,
		CreatedBy:   input.CreatedBy,
	}

	log.Printf("collectionService.Create: creating collection %s for tenant %s by user %s",
		collection.ID, input.TenantID, input.CreatedBy)

	if err := s.collectionRepo.Create(ctx, collection); err != nil {
		log.Printf("collectionService.Create: failed to create collection: %v", err)
		return nil, fmt.Errorf("creating collection: %w", err)
	}

	// Auto-assign owner permission to creator.
	// Note: if permission upsert fails, the collection exists without an owner (no transaction).
	// This matches the existing pattern for regular users below.
	if input.Role == domain.RoleService {
		// Service accounts use service_account_permissions table
		saPerm := &domain.ServiceAccountPermission{
			ServiceAccountID: input.CreatedBy,
			CollectionID:     collection.ID,
			TenantID:         input.TenantID,
			Permission:       domain.CollectionPermOwner,
			GrantedBy:        input.CreatedBy,
		}
		if err := s.saPermRepo.Upsert(ctx, saPerm); err != nil {
			log.Printf("collectionService.Create: failed to assign SA owner permission for collection %s: %v",
				collection.ID, err)
			return nil, fmt.Errorf("assigning owner permission: %w", err)
		}
	} else {
		// Regular users use collection_permissions table
		ownerPerm := &domain.CollectionPermissionEntry{
			CollectionID: collection.ID,
			TenantID:     input.TenantID,
			UserID:       input.CreatedBy,
			Permission:   domain.CollectionPermOwner,
			GrantedBy:    input.CreatedBy,
		}
		if err := s.permRepo.Upsert(ctx, ownerPerm); err != nil {
			log.Printf("collectionService.Create: failed to assign owner permission for collection %s: %v",
				collection.ID, err)
			return nil, fmt.Errorf("assigning owner permission: %w", err)
		}

		// If owner_email is provided, look up the user and grant them owner permission too.
		// This is non-blocking: if the user doesn't exist or the upsert fails, we log and continue.
		if input.OwnerEmail != "" {
			ownerUser, lookupErr := s.userRepo.GetByEmail(ctx, input.TenantID, input.OwnerEmail)
			if lookupErr != nil {
				log.Printf("collectionService.Create: owner_email %q not found in tenant %s, skipping: %v",
					input.OwnerEmail, input.TenantID, lookupErr)
			} else if ownerUser.ID != input.CreatedBy {
				emailOwnerPerm := &domain.CollectionPermissionEntry{
					CollectionID: collection.ID,
					TenantID:     input.TenantID,
					UserID:       ownerUser.ID,
					Permission:   domain.CollectionPermOwner,
					GrantedBy:    input.CreatedBy,
				}
				if upsertErr := s.permRepo.Upsert(ctx, emailOwnerPerm); upsertErr != nil {
					log.Printf("collectionService.Create: failed to grant owner permission to %s for collection %s: %v",
						ownerUser.ID, collection.ID, upsertErr)
				} else {
					log.Printf("collectionService.Create: granted owner permission to user %s (email: %s) for collection %s",
						ownerUser.ID, input.OwnerEmail, collection.ID)
				}
			}
		}
	}

	log.Printf("collectionService.Create: collection %s created successfully", collection.ID)
	return collection, nil
}

func (s *collectionService) GetByID(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole) (*domain.Collection, error) {
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermViewer); err != nil {
		return nil, err
	}
	return s.collectionRepo.GetByID(ctx, tenantID, collectionID)
}

func (s *collectionService) List(ctx context.Context, tenantID, userID uuid.UUID, role domain.UserRole, offset, limit int) ([]domain.Collection, int, error) {
	// Admin, manager, and member can see all collections in the tenant
	if role == domain.RoleAdmin || role == domain.RoleManager || role == domain.RoleMember {
		return s.collectionRepo.ListByTenant(ctx, tenantID, offset, limit)
	}
	// Service accounts see only collections they have explicit permission for (via service_account_permissions)
	if role == domain.RoleService {
		return s.collectionRepo.ListByServiceAccount(ctx, tenantID, userID, offset, limit)
	}
	// Viewer/free sees only collections they have explicit permission for (via collection_permissions)
	return s.collectionRepo.ListByUser(ctx, tenantID, userID, offset, limit)
}

func (s *collectionService) Update(ctx context.Context, input *UpdateCollectionInput) (*domain.Collection, error) {
	if err := s.requirePermission(ctx, input.CollectionID, input.UserID, input.Role, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	collection, err := s.collectionRepo.GetByID(ctx, input.TenantID, input.CollectionID)
	if err != nil {
		return nil, err
	}

	collection.Name = input.Name
	collection.Description = input.Description

	if err := s.collectionRepo.Update(ctx, collection); err != nil {
		return nil, err
	}

	return collection, nil
}

func (s *collectionService) Delete(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole) error {
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermOwner); err != nil {
		return err
	}
	log.Printf("collectionService.Delete: deleting collection %s by user %s", collectionID, userID)
	return s.collectionRepo.Delete(ctx, tenantID, collectionID)
}

func (s *collectionService) ListFiles(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, offset, limit int) ([]domain.FileMeta, int, error) {
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermViewer); err != nil {
		return nil, 0, err
	}
	return s.fileRepo.ListByCollection(ctx, tenantID, collectionID, offset, limit)
}

func (s *collectionService) BatchUploadFiles(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, files []BatchUploadFileInput) ([]BatchUploadResult, error) {
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	log.Printf("collectionService.BatchUploadFiles: uploading %d files to collection %s by user %s",
		len(files), collectionID, userID)

	results := make([]BatchUploadResult, 0, len(files))
	for _, f := range files {
		result := BatchUploadResult{FileName: f.Header.Filename}

		meta, err := s.fileSvc.Upload(ctx, FileUploadInput{
			TenantID:   tenantID,
			UploadedBy: userID,
			File:       f.File,
			Header:     f.Header,
		})
		if err != nil {
			log.Printf("collectionService.BatchUploadFiles: failed to upload file %s: %v",
				f.Header.Filename, err)
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		// Associate file with collection
		cf := &domain.CollectionFile{
			CollectionID: collectionID,
			FileID:       meta.ID,
			TenantID:     tenantID,
			AddedBy:      userID,
		}
		if err := s.fileRepo.Add(ctx, cf); err != nil {
			result.Error = fmt.Sprintf("uploaded but failed to add to collection: %s", err.Error())
			result.File = meta
			results = append(results, result)
			continue
		}

		result.Success = true
		result.File = meta
		results = append(results, result)
	}

	return results, nil
}

func (s *collectionService) RemoveFile(ctx context.Context, tenantID, collectionID, fileID, userID uuid.UUID, role domain.UserRole) error {
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return err
	}
	return s.fileRepo.Remove(ctx, collectionID, fileID)
}

func (s *collectionService) AddFileToCollection(ctx context.Context, tenantID, collectionID, fileID, userID uuid.UUID, role domain.UserRole) error {
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return err
	}
	cf := &domain.CollectionFile{
		CollectionID: collectionID,
		FileID:       fileID,
		TenantID:     tenantID,
		AddedBy:      userID,
	}
	return s.fileRepo.Add(ctx, cf)
}

func (s *collectionService) SetPermission(ctx context.Context, input *SetPermissionInput) error {
	// Service accounts cannot manage user-level collection permissions
	if input.CallerRole == domain.RoleService {
		return domain.ErrInsufficientRole
	}
	if !domain.ValidCollectionPermissions[input.Permission] {
		return domain.ErrInvalidPermission
	}
	if err := s.requirePermission(ctx, input.CollectionID, input.GrantedBy, input.CallerRole, domain.CollectionPermOwner); err != nil {
		return err
	}

	// Verify the target user exists in this tenant
	if _, err := s.userRepo.GetByID(ctx, input.TenantID, input.UserID); err != nil {
		return fmt.Errorf("target user not found in tenant: %w", domain.ErrNotFound)
	}

	log.Printf("collectionService.SetPermission: setting %s permission for user %s on collection %s by user %s",
		input.Permission, input.UserID, input.CollectionID, input.GrantedBy)

	perm := &domain.CollectionPermissionEntry{
		CollectionID: input.CollectionID,
		TenantID:     input.TenantID,
		UserID:       input.UserID,
		Permission:   input.Permission,
		GrantedBy:    input.GrantedBy,
	}
	return s.permRepo.Upsert(ctx, perm)
}

func (s *collectionService) ListPermissions(ctx context.Context, tenantID, collectionID, userID uuid.UUID, role domain.UserRole, offset, limit int) ([]domain.CollectionPermissionEntry, int, error) {
	// Service accounts cannot list user-level collection permissions
	if role == domain.RoleService {
		return nil, 0, domain.ErrInsufficientRole
	}
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermOwner); err != nil {
		return nil, 0, err
	}
	return s.permRepo.ListByCollection(ctx, collectionID, offset, limit)
}

func (s *collectionService) RemovePermission(ctx context.Context, tenantID, collectionID, targetUserID, userID uuid.UUID, role domain.UserRole) error {
	// Service accounts cannot manage user-level collection permissions
	if role == domain.RoleService {
		return domain.ErrInsufficientRole
	}
	if targetUserID == userID {
		return domain.ErrSelfPermissionRemoval
	}
	if err := s.requirePermission(ctx, collectionID, userID, role, domain.CollectionPermOwner); err != nil {
		return err
	}
	log.Printf("collectionService.RemovePermission: removing permission for user %s on collection %s by user %s",
		targetUserID, collectionID, userID)
	return s.permRepo.Delete(ctx, collectionID, targetUserID)
}

// EffectivePermission returns the effective collection permission for a user,
// combining their tenant role's implicit permission with any explicit grant.
func (s *collectionService) EffectivePermission(ctx context.Context, collectionID, userID uuid.UUID, role domain.UserRole) domain.CollectionPermission {
	return s.effectivePermission(ctx, collectionID, userID, role)
}

// EffectivePermissions returns the effective permission for a user across multiple collections.
// Optimized: admin always gets owner, viewer always gets viewer, manager/member do a single batch query.
func (s *collectionService) EffectivePermissions(ctx context.Context, collectionIDs []uuid.UUID, userID uuid.UUID, role domain.UserRole) (map[uuid.UUID]domain.CollectionPermission, error) {
	result := make(map[uuid.UUID]domain.CollectionPermission, len(collectionIDs))

	// Admin always has owner-level access
	if role == domain.RoleAdmin {
		for _, id := range collectionIDs {
			result[id] = domain.CollectionPermOwner
		}
		return result, nil
	}

	// Service accounts: batch-query from service_account_permissions (no implicit perms)
	if role == domain.RoleService {
		if s.saPermRepo == nil {
			return result, nil
		}
		return s.saPermRepo.GetByAccountForCollections(ctx, userID, collectionIDs)
	}

	// Manager/member/viewer: batch-query explicit permissions
	implicit := domain.ImplicitCollectionPerm(role)
	explicitPerms, err := s.permRepo.GetByUserForCollections(ctx, userID, collectionIDs)
	if err != nil {
		return nil, fmt.Errorf("computing effective permissions: %w", err)
	}

	for _, id := range collectionIDs {
		explicit, ok := explicitPerms[id]
		if ok && domain.CollectionPermLevel(explicit) > domain.CollectionPermLevel(implicit) {
			result[id] = explicit
		} else {
			result[id] = implicit
		}
	}
	return result, nil
}
