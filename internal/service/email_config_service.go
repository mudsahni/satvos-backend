package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// UpdateEmailConfigInput is the DTO for updating email processing config.
type UpdateEmailConfigInput struct {
	Enabled        *bool     `json:"enabled"`
	AllowedSenders *[]string `json:"allowed_senders"`
}

// EmailConfigOutput is the API response DTO for email processing config.
type EmailConfigOutput struct {
	TenantSlug        string   `json:"tenant_slug"`
	Enabled           bool     `json:"enabled"`
	AllowedSenders    []string `json:"allowed_senders"`
	APIBaseURL        string   `json:"api_base_url"`
	HasServiceAccount bool     `json:"has_service_account"`
}

// EmailConfigService defines the contract for managing tenant email processing config.
type EmailConfigService interface {
	GetConfig(ctx context.Context, tenantID uuid.UUID) (*EmailConfigOutput, error)
	UpdateConfig(ctx context.Context, tenantID, callerID uuid.UUID, input UpdateEmailConfigInput) (*EmailConfigOutput, error)
}

type emailConfigService struct {
	emailConfigRepo port.TenantEmailConfigRepository
	tenantRepo      port.TenantRepository
	saSvc           ServiceAccountService
	apiBaseURL      string
}

// NewEmailConfigService creates a new EmailConfigService implementation.
func NewEmailConfigService(
	emailConfigRepo port.TenantEmailConfigRepository,
	tenantRepo port.TenantRepository,
	saSvc ServiceAccountService,
	apiBaseURL string,
) EmailConfigService {
	return &emailConfigService{
		emailConfigRepo: emailConfigRepo,
		tenantRepo:      tenantRepo,
		saSvc:           saSvc,
		apiBaseURL:      apiBaseURL,
	}
}

func (s *emailConfigService) GetConfig(ctx context.Context, tenantID uuid.UUID) (*EmailConfigOutput, error) {
	tenant, err := s.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	cfg, err := s.emailConfigRepo.Get(ctx, tenant.Slug)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("getting email config: %w", err)
		}
		// Auto-create on first access
		if ensureErr := s.ensureEntry(ctx, tenant, uuid.Nil); ensureErr != nil {
			return nil, ensureErr
		}
		cfg, err = s.emailConfigRepo.Get(ctx, tenant.Slug)
		if err != nil {
			return nil, fmt.Errorf("getting email config after auto-create: %w", err)
		}
	}

	return toOutput(cfg), nil
}

func (s *emailConfigService) UpdateConfig(ctx context.Context, tenantID, callerID uuid.UUID, input UpdateEmailConfigInput) (*EmailConfigOutput, error) {
	tenant, err := s.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Ensure entry exists
	if err := s.ensureEntry(ctx, tenant, callerID); err != nil {
		return nil, err
	}

	// Validate allowed_senders if provided
	if input.AllowedSenders != nil {
		if err := validateAllowedSenders(*input.AllowedSenders); err != nil {
			return nil, err
		}
	}

	// Read current config to merge
	existing, err := s.emailConfigRepo.Get(ctx, tenant.Slug)
	if err != nil {
		return nil, fmt.Errorf("reading existing email config: %w", err)
	}

	// Merge non-nil fields
	mergedEnabled := existing.Enabled
	if input.Enabled != nil {
		mergedEnabled = *input.Enabled
	}
	mergedSenders := existing.AllowedSenders
	if input.AllowedSenders != nil {
		mergedSenders = *input.AllowedSenders
	}

	if err := s.emailConfigRepo.UpdateConfig(ctx, tenant.Slug, mergedEnabled, mergedSenders); err != nil {
		return nil, fmt.Errorf("updating email config: %w", err)
	}

	// Re-read and return
	cfg, err := s.emailConfigRepo.Get(ctx, tenant.Slug)
	if err != nil {
		return nil, fmt.Errorf("reading email config after update: %w", err)
	}

	return toOutput(cfg), nil
}

func (s *emailConfigService) ensureEntry(ctx context.Context, tenant *domain.Tenant, callerID uuid.UUID) error {
	_, err := s.emailConfigRepo.Get(ctx, tenant.Slug)
	if err == nil {
		return nil // already exists
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("checking email config: %w", err)
	}

	// Get or create the inbound_email SA and get raw key
	rawKey, err := s.saSvc.GetOrCreateInboundEmailKey(ctx, tenant.ID, callerID)
	if err != nil {
		return fmt.Errorf("getting inbound email API key: %w", err)
	}

	item := &port.TenantEmailConfig{
		TenantSlug:     tenant.Slug,
		ServiceAPIKey:  rawKey,
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     s.apiBaseURL,
	}

	if err := s.emailConfigRepo.Put(ctx, item); err != nil {
		return fmt.Errorf("creating email config entry: %w", err)
	}

	return nil
}

func toOutput(cfg *port.TenantEmailConfig) *EmailConfigOutput {
	senders := cfg.AllowedSenders
	if senders == nil {
		senders = []string{}
	}
	return &EmailConfigOutput{
		TenantSlug:        cfg.TenantSlug,
		Enabled:           cfg.Enabled,
		AllowedSenders:    senders,
		APIBaseURL:        cfg.APIBaseURL,
		HasServiceAccount: true,
	}
}

// validateAllowedSenders checks each entry is a valid email, @domain, or *.
func validateAllowedSenders(senders []string) error {
	for _, entry := range senders {
		if entry == "*" {
			continue
		}
		if strings.HasPrefix(entry, "@") {
			// Domain format: @domain.com
			d := entry[1:]
			if !strings.Contains(d, ".") || len(d) < 3 {
				return fmt.Errorf("%w: %q is not a valid domain", domain.ErrInvalidAllowedSender, entry)
			}
			continue
		}
		// Must be a valid email
		if _, err := mail.ParseAddress(entry); err != nil {
			return fmt.Errorf("%w: %q is not a valid email address", domain.ErrInvalidAllowedSender, entry)
		}
	}
	return nil
}
