package port

import "context"

// TenantEmailConfig represents a tenant's email processing configuration stored in DynamoDB.
type TenantEmailConfig struct {
	TenantSlug     string   `json:"tenant_slug"`
	ServiceAPIKey  string   `json:"-"`
	Enabled        bool     `json:"enabled"`
	AllowedSenders []string `json:"allowed_senders"`
	APIBaseURL     string   `json:"api_base_url"`
	InboundAddress string   `json:"inbound_address"`
}

// TenantEmailConfigRepository defines operations for managing tenant email processing config in DynamoDB.
type TenantEmailConfigRepository interface {
	Get(ctx context.Context, tenantSlug string) (*TenantEmailConfig, error)
	Put(ctx context.Context, item *TenantEmailConfig) error
	UpdateConfig(ctx context.Context, tenantSlug string, enabled bool, allowedSenders []string) error
}
