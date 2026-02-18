package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

const (
	apiKeyLength = 32 // 32 bytes = 64 hex chars
	apiKeyPrefix = "sk_"
)

// CreateServiceAccountInput is the DTO for creating a service account.
type CreateServiceAccountInput struct {
	TenantID    uuid.UUID
	Name        string
	Description string
	ExpiresAt   *time.Time
	CreatedBy   uuid.UUID
}

// CreateServiceAccountOutput holds the created account and the raw API key (shown only once).
type CreateServiceAccountOutput struct {
	ServiceAccount *domain.ServiceAccount `json:"service_account"`
	APIKey         string                 `json:"api_key"` // raw key, shown only at creation
}

// RotateAPIKeyOutput holds the updated account and the new raw API key.
type RotateAPIKeyOutput struct {
	ServiceAccount *domain.ServiceAccount `json:"service_account"`
	APIKey         string                 `json:"api_key"`
}

// SASetPermissionInput is the DTO for granting a service account access to a collection.
type SASetPermissionInput struct {
	TenantID         uuid.UUID
	ServiceAccountID uuid.UUID
	CollectionID     uuid.UUID
	Permission       domain.CollectionPermission
	GrantedBy        uuid.UUID
}

// ServiceAccountService defines the service account management contract.
type ServiceAccountService interface {
	Create(ctx context.Context, input *CreateServiceAccountInput) (*CreateServiceAccountOutput, error)
	GetByID(ctx context.Context, tenantID, saID uuid.UUID) (*domain.ServiceAccount, error)
	List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.ServiceAccount, int, error)
	RotateAPIKey(ctx context.Context, tenantID, saID uuid.UUID) (*RotateAPIKeyOutput, error)
	Revoke(ctx context.Context, tenantID, saID uuid.UUID) error
	Delete(ctx context.Context, tenantID, saID uuid.UUID) error

	// Permission management
	SetPermission(ctx context.Context, input *SASetPermissionInput) error
	ListPermissions(ctx context.Context, tenantID, saID uuid.UUID) ([]domain.ServiceAccountPermission, error)
	RemovePermission(ctx context.Context, tenantID, saID, collectionID uuid.UUID) error

	// Authentication
	Authenticate(ctx context.Context, rawKey string) (*domain.ServiceAccount, error)
}

type serviceAccountService struct {
	saRepo         port.ServiceAccountRepository
	permRepo       port.ServiceAccountPermissionRepository
	collectionRepo port.CollectionRepository
	lastUsedCache  sync.Map // uuid.UUID -> time.Time — debounce UpdateLastUsed writes
}

// NewServiceAccountService creates a new ServiceAccountService implementation.
func NewServiceAccountService(
	saRepo port.ServiceAccountRepository,
	permRepo port.ServiceAccountPermissionRepository,
	collectionRepo port.CollectionRepository,
) ServiceAccountService {
	return &serviceAccountService{
		saRepo:         saRepo,
		permRepo:       permRepo,
		collectionRepo: collectionRepo,
	}
}

func (s *serviceAccountService) Create(ctx context.Context, input *CreateServiceAccountInput) (*CreateServiceAccountOutput, error) {
	rawKey, keyHash, prefix, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generating API key: %w", err)
	}

	sa := &domain.ServiceAccount{
		TenantID:     input.TenantID,
		Name:         input.Name,
		Description:  input.Description,
		APIKeyPrefix: prefix,
		APIKeyHash:   keyHash,
		IsActive:     true,
		CreatedBy:    input.CreatedBy,
		ExpiresAt:    input.ExpiresAt,
	}

	if err := s.saRepo.Create(ctx, sa); err != nil {
		return nil, err
	}

	return &CreateServiceAccountOutput{
		ServiceAccount: sa,
		APIKey:         rawKey,
	}, nil
}

func (s *serviceAccountService) GetByID(ctx context.Context, tenantID, saID uuid.UUID) (*domain.ServiceAccount, error) {
	return s.saRepo.GetByID(ctx, tenantID, saID)
}

func (s *serviceAccountService) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.ServiceAccount, int, error) {
	return s.saRepo.ListByTenant(ctx, tenantID, offset, limit)
}

func (s *serviceAccountService) RotateAPIKey(ctx context.Context, tenantID, saID uuid.UUID) (*RotateAPIKeyOutput, error) {
	rawKey, keyHash, prefix, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generating API key: %w", err)
	}

	// Atomic update — only succeeds if the SA exists, belongs to tenant, and is active
	if err := s.saRepo.RotateKey(ctx, tenantID, saID, prefix, keyHash); err != nil {
		return nil, err
	}

	// Re-fetch to return updated state
	sa, err := s.saRepo.GetByID(ctx, tenantID, saID)
	if err != nil {
		return nil, err
	}

	return &RotateAPIKeyOutput{
		ServiceAccount: sa,
		APIKey:         rawKey,
	}, nil
}

func (s *serviceAccountService) Revoke(ctx context.Context, tenantID, saID uuid.UUID) error {
	return s.saRepo.Revoke(ctx, tenantID, saID)
}

func (s *serviceAccountService) Delete(ctx context.Context, tenantID, saID uuid.UUID) error {
	return s.saRepo.Delete(ctx, tenantID, saID)
}

func (s *serviceAccountService) SetPermission(ctx context.Context, input *SASetPermissionInput) error {
	// Verify the service account exists and belongs to the tenant
	if _, err := s.saRepo.GetByID(ctx, input.TenantID, input.ServiceAccountID); err != nil {
		return err
	}

	// Verify the collection belongs to the same tenant
	if s.collectionRepo != nil {
		if _, err := s.collectionRepo.GetByID(ctx, input.TenantID, input.CollectionID); err != nil {
			return fmt.Errorf("collection not found in tenant: %w", domain.ErrCollectionNotFound)
		}
	}

	perm := &domain.ServiceAccountPermission{
		ServiceAccountID: input.ServiceAccountID,
		CollectionID:     input.CollectionID,
		TenantID:         input.TenantID,
		Permission:       input.Permission,
		GrantedBy:        input.GrantedBy,
	}
	return s.permRepo.Upsert(ctx, perm)
}

func (s *serviceAccountService) ListPermissions(ctx context.Context, tenantID, saID uuid.UUID) ([]domain.ServiceAccountPermission, error) {
	// Verify the service account exists and belongs to the tenant
	if _, err := s.saRepo.GetByID(ctx, tenantID, saID); err != nil {
		return nil, err
	}
	return s.permRepo.ListByAccount(ctx, saID)
}

func (s *serviceAccountService) RemovePermission(ctx context.Context, tenantID, saID, collectionID uuid.UUID) error {
	if _, err := s.saRepo.GetByID(ctx, tenantID, saID); err != nil {
		return err
	}
	return s.permRepo.Delete(ctx, saID, collectionID)
}

func (s *serviceAccountService) Authenticate(ctx context.Context, rawKey string) (*domain.ServiceAccount, error) {
	// Extract prefix from key: "sk_<8-char-prefix><rest>"
	if len(rawKey) < len(apiKeyPrefix)+8 {
		return nil, domain.ErrAPIKeyInvalid
	}
	if rawKey[:len(apiKeyPrefix)] != apiKeyPrefix {
		return nil, domain.ErrAPIKeyInvalid
	}

	prefix := rawKey[len(apiKeyPrefix) : len(apiKeyPrefix)+8]
	candidates, err := s.saRepo.GetByAPIKeyPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("looking up API key: %w", err)
	}
	if len(candidates) == 0 {
		return nil, domain.ErrAPIKeyInvalid
	}

	keyHash := hashAPIKey(rawKey)

	for i := range candidates {
		if subtle.ConstantTimeCompare([]byte(candidates[i].APIKeyHash), []byte(keyHash)) != 1 {
			continue
		}

		sa := &candidates[i]

		// Defense-in-depth: verify active state in code (SQL already filters is_active=TRUE)
		if !sa.IsActive {
			return nil, domain.ErrAPIKeyRevoked
		}

		// Check expiry
		if sa.ExpiresAt != nil && sa.ExpiresAt.Before(time.Now()) {
			return nil, domain.ErrAPIKeyRevoked
		}

		// Update last used (non-blocking, debounced to 60s per SA)
		now := time.Now()
		if prev, loaded := s.lastUsedCache.Load(sa.ID); !loaded ||
			now.Sub(prev.(time.Time)) > 60*time.Second {
			s.lastUsedCache.Store(sa.ID, now)
			go func(id uuid.UUID) {
				if updateErr := s.saRepo.UpdateLastUsed(context.Background(), id); updateErr != nil {
					log.Printf("WARNING: failed to update service account last_used_at: %v", updateErr)
				}
			}(sa.ID)
		}

		return sa, nil
	}

	return nil, domain.ErrAPIKeyInvalid
}

// generateAPIKey creates a new random API key and returns (rawKey, sha256Hash, prefix).
func generateAPIKey() (raw, hash, prefix string, err error) {
	b := make([]byte, apiKeyLength)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generating random bytes: %w", err)
	}

	hexKey := hex.EncodeToString(b)
	raw = apiKeyPrefix + hexKey
	prefix = hexKey[:8]
	hash = hashAPIKey(raw)
	return raw, hash, prefix, nil
}

// hashAPIKey returns the SHA-256 hex digest of a raw API key.
func hashAPIKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
