package handler

import (
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
)

// Swagger type definitions for API documentation.
// These types are used by swag to generate OpenAPI documentation.

// --- Request Types ---

// LoginRequest represents the login request body.
type LoginRequest struct {
	TenantSlug string `json:"tenant_slug" binding:"required" example:"acme"`
	Email      string `json:"email" binding:"required" example:"admin@acme.com"`
	Password   string `json:"password" binding:"required" example:"securepassword123"`
}

// RefreshRequest represents the token refresh request body.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// CreateCollectionRequest represents the create collection request body.
type CreateCollectionRequest struct {
	Name        string `json:"name" binding:"required" example:"Q4 2024 Invoices"`
	Description string `json:"description" example:"Invoices from Q4 2024 fiscal quarter"`
}

// UpdateCollectionRequest represents the update collection request body.
type UpdateCollectionRequest struct {
	Name        string `json:"name" binding:"required" example:"Q4 2024 Invoices - Final"`
	Description string `json:"description" example:"Updated description"`
}

// SetPermissionRequest represents the set permission request body.
type SetPermissionRequest struct {
	UserID     uuid.UUID                   `json:"user_id" binding:"required" example:"987fcdeb-51a2-3bc4-d567-890123456789"`
	Permission domain.CollectionPermission `json:"permission" binding:"required" example:"editor"`
}

// CreateDocumentRequest represents the create document request body.
type CreateDocumentRequest struct {
	FileID       uuid.UUID         `json:"file_id" binding:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	CollectionID uuid.UUID         `json:"collection_id" binding:"required" example:"660e8400-e29b-41d4-a716-446655440001"`
	DocumentType string            `json:"document_type" binding:"required" example:"invoice"`
	ParseMode    domain.ParseMode  `json:"parse_mode" example:"single"`
	Name         string            `json:"name" example:"Acme Corp Invoice Q4-2024"`
	Tags         map[string]string `json:"tags" example:"vendor:Acme Corp,quarter:Q4"`
}

// ReviewDocumentRequest represents the review document request body.
type ReviewDocumentRequest struct {
	Status string `json:"status" binding:"required" example:"approved"`
	Notes  string `json:"notes" example:"Verified against source PDF. All data correct."`
}

// EditStructuredDataRequest represents the edit structured data request body.
type EditStructuredDataRequest struct {
	StructuredData GSTInvoice `json:"structured_data" binding:"required"`
}

// AddTagsRequest represents the add tags request body.
type AddTagsRequest struct {
	Tags map[string]string `json:"tags" binding:"required" example:"department:Engineering,cost_center:CC-1234"`
}

// CreateUserRequest represents the create user request body.
type CreateUserRequest struct {
	Email    string          `json:"email" binding:"required" example:"jane.doe@acme.com"`
	Password string          `json:"password" binding:"required" example:"securepassword123"`
	FullName string          `json:"full_name" example:"Jane Doe"`
	Role     domain.UserRole `json:"role" binding:"required" example:"member"`
}

// UpdateUserRequest represents the update user request body.
type UpdateUserRequest struct {
	Email    *string          `json:"email" example:"jane.smith@acme.com"`
	FullName *string          `json:"full_name" example:"Jane Smith"`
	Role     *domain.UserRole `json:"role" example:"manager"`
	IsActive *bool            `json:"is_active" example:"true"`
}

// CreateTenantRequest represents the create tenant request body.
type CreateTenantRequest struct {
	Name string `json:"name" binding:"required" example:"Acme Corporation"`
	Slug string `json:"slug" binding:"required" example:"acme"`
}

// UpdateTenantRequest represents the update tenant request body.
type UpdateTenantRequest struct {
	Name     *string `json:"name" example:"Acme Industries"`
	Slug     *string `json:"slug" example:"acme-ind"`
	IsActive *bool   `json:"is_active" example:"false"`
}

// RegisterRequest represents the free-tier registration request body.
type RegisterRequest struct {
	Email    string `json:"email" binding:"required" example:"user@example.com"`
	Password string `json:"password" binding:"required" example:"securepassword123"`
	FullName string `json:"full_name" binding:"required" example:"John Doe"`
}

// ForgotPasswordRequest represents the forgot-password request body.
type ForgotPasswordRequest struct {
	TenantSlug string `json:"tenant_slug" binding:"required" example:"satvos"`
	Email      string `json:"email" binding:"required" example:"user@example.com"`
}

// ResetPasswordRequest represents the reset-password request body.
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	NewPassword string `json:"new_password" binding:"required" example:"newsecurepassword456"`
}

// SocialLoginRequest represents the social login request body.
type SocialLoginRequest struct {
	Provider string `json:"provider" binding:"required" example:"google"`
	IDToken  string `json:"id_token" binding:"required" example:"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// --- Response Types ---

// RegisterResponse represents the registration response.
type RegisterResponse struct {
	User       domain.User       `json:"user"`
	Collection domain.Collection `json:"collection"`
	Tokens     TokenResponse     `json:"tokens"`
}

// SocialLoginResponse represents the social login response.
type SocialLoginResponse struct {
	User       domain.User        `json:"user"`
	Collection *domain.Collection `json:"collection,omitempty"`
	Tokens     TokenResponse      `json:"tokens"`
	IsNewUser  bool               `json:"is_new_user" example:"false"`
}

// TokenResponse represents the authentication token response.
type TokenResponse struct {
	AccessToken  string    `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string    `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresAt    time.Time `json:"expires_at" example:"2025-01-15T10:30:00Z"`
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
	Error  string `json:"error,omitempty" example:"database not reachable"`
}

// MessageResponse represents a simple message response.
type MessageResponse struct {
	Message string `json:"message" example:"operation completed successfully"`
}

// FileWithDownloadURL represents a file with its download URL.
type FileWithDownloadURL struct {
	File        domain.FileMeta `json:"file"`
	DownloadURL string          `json:"download_url" example:"https://s3.amazonaws.com/satvos-uploads/...?X-Amz-Signature=..."`
}

// FileUploadWithWarning represents a file upload response with optional warning.
type FileUploadWithWarning struct {
	File    domain.FileMeta `json:"file"`
	Warning string          `json:"warning,omitempty" example:"file uploaded but could not be added to collection"`
}

// CollectionWithFiles represents a collection with its files.
type CollectionWithFiles struct {
	Collection domain.Collection `json:"collection"`
	Files      []domain.FileMeta `json:"files"`
	FilesMeta  PagMeta           `json:"files_meta"`
}

// BatchUploadResult represents the result of a single file in batch upload.
type BatchUploadResult struct {
	File    *domain.FileMeta `json:"file"`
	Success bool             `json:"success" example:"true"`
	Error   *string          `json:"error" example:"unsupported file type"`
}

// ValidationSummary represents validation summary statistics.
type ValidationSummary struct {
	Total    int `json:"total" example:"50"`
	Passed   int `json:"passed" example:"48"`
	Errors   int `json:"errors" example:"0"`
	Warnings int `json:"warnings" example:"2"`
}

// ValidationResultEntry represents a single validation rule result.
type ValidationResultEntry struct {
	RuleName              string `json:"rule_name" example:"Required: Invoice Number"`
	RuleType              string `json:"rule_type" example:"required_field"`
	Severity              string `json:"severity" example:"error"`
	Passed                bool   `json:"passed" example:"true"`
	FieldPath             string `json:"field_path" example:"invoice.invoice_number"`
	ExpectedValue         string `json:"expected_value" example:"non-empty value"`
	ActualValue           string `json:"actual_value" example:"INV-2024-001234"`
	Message               string `json:"message" example:"Required: Invoice Number: invoice.invoice_number is present"`
	ReconciliationCritical bool  `json:"reconciliation_critical" example:"true"`
}

// FieldStatus represents the validation status of a single field.
type FieldStatus struct {
	Status   string   `json:"status" example:"valid"`
	Messages []string `json:"messages"`
}

// ValidationResponse represents the full validation results for a document.
type ValidationResponse struct {
	DocumentID             uuid.UUID                `json:"document_id" example:"880e8400-e29b-41d4-a716-446655440003"`
	ValidationStatus       string                   `json:"validation_status" example:"warning"`
	Summary                ValidationSummary        `json:"summary"`
	ReconciliationStatus   string                   `json:"reconciliation_status" example:"valid"`
	ReconciliationSummary  ValidationSummary        `json:"reconciliation_summary"`
	Results                []ValidationResultEntry  `json:"results"`
	FieldStatuses          map[string]FieldStatus   `json:"field_statuses"`
}

// --- Generic Response Wrappers ---

// Response wraps a successful response with data.
type Response struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data,omitempty"`
	Meta    *PagMeta    `json:"meta,omitempty"`
}

// ErrorResponse wraps an error response.
type ErrorResponseBody struct {
	Success bool      `json:"success" example:"false"`
	Error   *APIError `json:"error"`
}

// --- Parsed Invoice Schema (for documentation) ---

// GSTInvoice represents the full parsed invoice structure.
type GSTInvoice struct {
	Invoice  InvoiceHeader `json:"invoice"`
	Seller   Party         `json:"seller"`
	Buyer    Party         `json:"buyer"`
	LineItems []LineItem   `json:"line_items"`
	Totals   Totals        `json:"totals"`
	Payment  Payment       `json:"payment"`
	Notes    string        `json:"notes" example:"Thank you for your business"`
}

// InvoiceHeader represents invoice header fields.
type InvoiceHeader struct {
	InvoiceNumber string `json:"invoice_number" example:"INV-2024-001234"`
	InvoiceDate   string `json:"invoice_date" example:"2024-12-15"`
	DueDate       string `json:"due_date" example:"2025-01-15"`
	InvoiceType   string `json:"invoice_type" example:"tax_invoice"`
	Currency      string `json:"currency" example:"INR"`
	PlaceOfSupply string `json:"place_of_supply" example:"Karnataka"`
	ReverseCharge bool   `json:"reverse_charge" example:"false"`
}

// Party represents seller or buyer information.
type Party struct {
	Name      string `json:"name" example:"Acme Corporation Pvt Ltd"`
	Address   string `json:"address" example:"123 Tech Park, Bangalore"`
	GSTIN     string `json:"gstin" example:"29AABCU9603R1ZM"`
	PAN       string `json:"pan" example:"AABCU9603R"`
	State     string `json:"state" example:"Karnataka"`
	StateCode string `json:"state_code" example:"29"`
}

// LineItem represents a single line item in the invoice.
type LineItem struct {
	Description   string  `json:"description" example:"Software Development Services"`
	HSNSACCode    string  `json:"hsn_sac_code" example:"998314"`
	Quantity      float64 `json:"quantity" example:"100"`
	Unit          string  `json:"unit" example:"hours"`
	UnitPrice     float64 `json:"unit_price" example:"2500.00"`
	Discount      float64 `json:"discount" example:"5000.00"`
	TaxableAmount float64 `json:"taxable_amount" example:"245000.00"`
	CGSTRate      float64 `json:"cgst_rate" example:"9"`
	CGSTAmount    float64 `json:"cgst_amount" example:"22050.00"`
	SGSTRate      float64 `json:"sgst_rate" example:"9"`
	SGSTAmount    float64 `json:"sgst_amount" example:"22050.00"`
	IGSTRate      float64 `json:"igst_rate" example:"0"`
	IGSTAmount    float64 `json:"igst_amount" example:"0"`
	Total         float64 `json:"total" example:"289100.00"`
}

// Totals represents invoice totals.
type Totals struct {
	Subtotal      float64 `json:"subtotal" example:"250000.00"`
	TotalDiscount float64 `json:"total_discount" example:"5000.00"`
	TaxableAmount float64 `json:"taxable_amount" example:"245000.00"`
	CGST          float64 `json:"cgst" example:"22050.00"`
	SGST          float64 `json:"sgst" example:"22050.00"`
	IGST          float64 `json:"igst" example:"0"`
	Cess          float64 `json:"cess" example:"0"`
	RoundOff      float64 `json:"round_off" example:"0"`
	Total         float64 `json:"total" example:"289100.00"`
	AmountInWords string  `json:"amount_in_words" example:"Two Lakh Eighty Nine Thousand One Hundred Rupees Only"`
}

// Payment represents payment details.
type Payment struct {
	BankName      string `json:"bank_name" example:"HDFC Bank"`
	AccountNumber string `json:"account_number" example:"50100123456789"`
	IFSCCode      string `json:"ifsc_code" example:"HDFC0001234"`
	PaymentTerms  string `json:"payment_terms" example:"Net 30"`
}
