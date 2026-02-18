package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
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
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	ExpiresAt   *time.Time `json:"expires_at"`
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
	ListPermissions(ctx context.Context, tenantID, saID uuid.UUID) ([]port.ServiceAccountPermission, error)
	RemovePermission(ctx context.Context, tenantID, saID, collectionID uuid.UUID) error

	// Authentication
	Authenticate(ctx context.Context, rawKey string) (*domain.ServiceAccount, error)
}

type serviceAccountService struct {
	saRepo   port.ServiceAccountRepository
	permRepo port.ServiceAccountPermissionRepository
}

// NewServiceAccountService creates a new ServiceAccountService implementation.
func NewServiceAccountService(
	saRepo port.ServiceAccountRepository,
	permRepo port.ServiceAccountPermissionRepository,
) ServiceAccountService {
	return &serviceAccountService{
		saRepo:   saRepo,
		permRepo: permRepo,
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
	sa, err := s.saRepo.GetByID(ctx, tenantID, saID)
	if err != nil {
		return nil, err
	}

	rawKey, keyHash, prefix, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generating API key: %w", err)
	}

	sa.APIKeyHash = keyHash
	sa.APIKeyPrefix = prefix

	if err := s.saRepo.Update(ctx, sa); err != nil {
		return nil, err
	}

	return &RotateAPIKeyOutput{
		ServiceAccount: sa,
		APIKey:         rawKey,
	}, nil
}

func (s *serviceAccountService) Revoke(ctx context.Context, tenantID, saID uuid.UUID) error {
	sa, err := s.saRepo.GetByID(ctx, tenantID, saID)
	if err != nil {
		return err
	}

	sa.IsActive = false
	return s.saRepo.Update(ctx, sa)
}

func (s *serviceAccountService) Delete(ctx context.Context, tenantID, saID uuid.UUID) error {
	return s.saRepo.Delete(ctx, tenantID, saID)
}

func (s *serviceAccountService) SetPermission(ctx context.Context, input *SASetPermissionInput) error {
	// Verify the service account exists and belongs to the tenant
	if _, err := s.saRepo.GetByID(ctx, input.TenantID, input.ServiceAccountID); err != nil {
		return err
	}

	perm := &port.ServiceAccountPermission{
		ServiceAccountID: input.ServiceAccountID,
		CollectionID:     input.CollectionID,
		TenantID:         input.TenantID,
		Permission:       input.Permission,
		GrantedBy:        input.GrantedBy,
	}
	return s.permRepo.Upsert(ctx, perm)
}

func (s *serviceAccountService) ListPermissions(ctx context.Context, tenantID, saID uuid.UUID) ([]port.ServiceAccountPermission, error) {
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
		if subtle.ConstantTimeCompare([]byte(candidates[i].APIKeyHash), []byte(keyHash)) == 1 {
			sa := &candidates[i]

			// Check expiry
			if sa.ExpiresAt != nil && sa.ExpiresAt.Before(time.Now()) {
				return nil, domain.ErrAPIKeyRevoked
			}

			// Update last used (non-blocking)
			go func(id uuid.UUID) {
				if updateErr := s.saRepo.UpdateLastUsed(context.Background(), id); updateErr != nil {
					log.Printf("WARNING: failed to update service account last_used_at: %v", updateErr)
				}
			}(sa.ID)

			return sa, nil
		}
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
