package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
)

// domainRegexp validates the part after @ in domain entries.
// Requires: starts with alnum, allows hyphens, at least one dot, TLD >= 2 chars.
var domainRegexp = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`)

// UpdateEmailConfigInput is the DTO for updating email processing config.
type UpdateEmailConfigInput struct {
	Enabled        *bool     `json:"enabled"`
	AllowedSenders *[]string `json:"allowed_senders"`
}

// EmailConfigServiceAccount is a summary of the service account tied to email processing.
type EmailConfigServiceAccount struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	IsActive     bool      `json:"is_active"`
	APIKeyPrefix string    `json:"api_key_prefix"`
}

// EmailConfigOutput is the API response DTO for email processing config.
type EmailConfigOutput struct {
	TenantSlug     string                     `json:"tenant_slug"`
	Enabled        bool                       `json:"enabled"`
	AllowedSenders []string                   `json:"allowed_senders"`
	APIBaseURL     string                     `json:"api_base_url"`
	InboundAddress string                     `json:"inbound_address,omitempty"`
	ServiceAccount *EmailConfigServiceAccount `json:"service_account,omitempty"`
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

	cfg, err := s.getOrCreateEntry(ctx, tenant, uuid.Nil)
	if err != nil {
		return nil, err
	}

	out := emailConfigToOutput(cfg)
	out.ServiceAccount = s.lookupInboundSA(ctx, tenantID)
	return out, nil
}

func (s *emailConfigService) UpdateConfig(ctx context.Context, tenantID, callerID uuid.UUID, input UpdateEmailConfigInput) (*EmailConfigOutput, error) {
	tenant, err := s.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Ensure entry exists (returns current config)
	existing, err := s.getOrCreateEntry(ctx, tenant, callerID)
	if err != nil {
		return nil, err
	}

	// Validate and normalize allowed_senders if provided
	if input.AllowedSenders != nil {
		normalized, valErr := validateAndNormalizeAllowedSenders(*input.AllowedSenders)
		if valErr != nil {
			return nil, valErr
		}
		input.AllowedSenders = &normalized
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

	out := emailConfigToOutput(cfg)
	out.ServiceAccount = s.lookupInboundSA(ctx, tenantID)
	return out, nil
}

// getOrCreateEntry returns the existing config or auto-creates it.
// Reduces redundant DynamoDB reads by returning the config directly.
func (s *emailConfigService) getOrCreateEntry(ctx context.Context, tenant *domain.Tenant, callerID uuid.UUID) (*port.TenantEmailConfig, error) {
	cfg, err := s.emailConfigRepo.Get(ctx, tenant.Slug)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("getting email config: %w", err)
	}

	// Get or create the inbound_email SA and get raw key
	rawKey, err := s.saSvc.GetOrCreateInboundEmailKey(ctx, tenant.ID, callerID)
	if err != nil {
		return nil, fmt.Errorf("getting inbound email API key: %w", err)
	}

	item := &port.TenantEmailConfig{
		TenantSlug:     tenant.Slug,
		ServiceAPIKey:  rawKey,
		Enabled:        false,
		AllowedSenders: []string{},
		APIBaseURL:     s.apiBaseURL,
	}

	if err := s.emailConfigRepo.Put(ctx, item); err != nil {
		return nil, fmt.Errorf("creating email config entry: %w", err)
	}

	// Re-read to return the stored state (Put is last-writer-wins, so read back what's there)
	cfg, err = s.emailConfigRepo.Get(ctx, tenant.Slug)
	if err != nil {
		return nil, fmt.Errorf("reading email config after create: %w", err)
	}

	return cfg, nil
}

// lookupInboundSA returns a summary of the inbound_email SA, or nil if not found.
func (s *emailConfigService) lookupInboundSA(ctx context.Context, tenantID uuid.UUID) *EmailConfigServiceAccount {
	sa, err := s.saSvc.GetInboundEmailAccount(ctx, tenantID)
	if err != nil {
		return nil
	}
	return &EmailConfigServiceAccount{
		ID:           sa.ID,
		Name:         sa.Name,
		IsActive:     sa.IsActive,
		APIKeyPrefix: sa.APIKeyPrefix,
	}
}

func emailConfigToOutput(cfg *port.TenantEmailConfig) *EmailConfigOutput {
	senders := cfg.AllowedSenders
	if senders == nil {
		senders = []string{}
	}
	return &EmailConfigOutput{
		TenantSlug:     cfg.TenantSlug,
		Enabled:        cfg.Enabled,
		AllowedSenders: senders,
		APIBaseURL:     cfg.APIBaseURL,
		InboundAddress: cfg.InboundAddress,
	}
}

// validateAndNormalizeAllowedSenders validates, trims, lowercases, and deduplicates entries.
// Each entry must be: "*", "@domain.com", or "user@domain.com".
func validateAndNormalizeAllowedSenders(senders []string) ([]string, error) {
	seen := make(map[string]struct{}, len(senders))
	normalized := make([]string, 0, len(senders))

	for _, raw := range senders {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if entry == "" {
			return nil, fmt.Errorf("%w: empty entry not allowed", domain.ErrInvalidAllowedSender)
		}

		if entry == "*" {
			if _, dup := seen[entry]; !dup {
				seen[entry] = struct{}{}
				normalized = append(normalized, entry)
			}
			continue
		}

		if strings.HasPrefix(entry, "@") {
			// Domain format: @domain.com
			d := entry[1:]
			if !domainRegexp.MatchString(d) {
				return nil, fmt.Errorf("%w: %q is not a valid domain", domain.ErrInvalidAllowedSender, raw)
			}
			if _, dup := seen[entry]; !dup {
				seen[entry] = struct{}{}
				normalized = append(normalized, entry)
			}
			continue
		}

		// Must be a valid email — reject display-name forms like "Name <email>"
		addr, err := mail.ParseAddress(entry)
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a valid email address", domain.ErrInvalidAllowedSender, raw)
		}
		if addr.Address != entry {
			return nil, fmt.Errorf("%w: %q must be a plain email address (no display names)", domain.ErrInvalidAllowedSender, raw)
		}

		if _, dup := seen[entry]; !dup {
			seen[entry] = struct{}{}
			normalized = append(normalized, entry)
		}
	}

	return normalized, nil
}
