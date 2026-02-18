package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Tenant represents an isolated organizational tenant.
type Tenant struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Slug      string    `db:"slug" json:"slug"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// User represents an authenticated user belonging to a tenant.
type User struct {
	ID                      uuid.UUID `db:"id" json:"id"`
	TenantID                uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Email                   string    `db:"email" json:"email"`
	PasswordHash            string    `db:"password_hash" json:"-"`
	FullName                string    `db:"full_name" json:"full_name"`
	Role                    UserRole  `db:"role" json:"role"`
	IsActive                bool      `db:"is_active" json:"is_active"`
	MonthlyDocumentLimit    int        `db:"monthly_document_limit" json:"monthly_document_limit"`
	DocumentsUsedThisPeriod int        `db:"documents_used_this_period" json:"documents_used_this_period"`
	CurrentPeriodStart      time.Time  `db:"current_period_start" json:"current_period_start"`
	EmailVerified           bool       `db:"email_verified" json:"email_verified"`
	EmailVerifiedAt         *time.Time `db:"email_verified_at" json:"email_verified_at,omitempty"`
	PasswordResetTokenID    *string      `db:"password_reset_token_id" json:"-"`
	AuthProvider            AuthProvider `db:"auth_provider" json:"auth_provider"`
	ProviderUserID          *string      `db:"provider_user_id" json:"-"`
	CreatedAt               time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt               time.Time    `db:"updated_at" json:"updated_at"`
}

// Collection represents a grouping of files within a tenant.
type Collection struct {
	ID            uuid.UUID `db:"id" json:"id"`
	TenantID      uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name          string    `db:"name" json:"name"`
	Description   string    `db:"description" json:"description"`
	CreatedBy     uuid.UUID `db:"created_by" json:"created_by"`
	DocumentCount int       `db:"document_count" json:"document_count"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// CollectionPermissionEntry represents a user's permission on a collection.
type CollectionPermissionEntry struct {
	ID           uuid.UUID            `db:"id" json:"id"`
	CollectionID uuid.UUID            `db:"collection_id" json:"collection_id"`
	TenantID     uuid.UUID            `db:"tenant_id" json:"tenant_id"`
	UserID       uuid.UUID            `db:"user_id" json:"user_id"`
	Permission   CollectionPermission `db:"permission" json:"permission"`
	GrantedBy    uuid.UUID            `db:"granted_by" json:"granted_by"`
	CreatedAt    time.Time            `db:"created_at" json:"created_at"`
}

// CollectionFile represents the association between a collection and a file.
type CollectionFile struct {
	CollectionID uuid.UUID `db:"collection_id" json:"collection_id"`
	FileID       uuid.UUID `db:"file_id" json:"file_id"`
	TenantID     uuid.UUID `db:"tenant_id" json:"tenant_id"`
	AddedBy      uuid.UUID `db:"added_by" json:"added_by"`
	AddedAt      time.Time `db:"added_at" json:"added_at"`
}

// Document represents a parsed document linked to an uploaded file.
type Document struct {
	ID               uuid.UUID          `db:"id" json:"id"`
	TenantID         uuid.UUID          `db:"tenant_id" json:"tenant_id"`
	CollectionID     uuid.UUID          `db:"collection_id" json:"collection_id"`
	FileID           uuid.UUID          `db:"file_id" json:"file_id"`
	Name             string             `db:"name" json:"name"`
	DocumentType     string             `db:"document_type" json:"document_type"`
	ParserModel      string             `db:"parser_model" json:"parser_model"`
	ParserPrompt     string             `db:"parser_prompt" json:"parser_prompt"`
	StructuredData   json.RawMessage    `db:"structured_data" json:"structured_data" swaggertype:"object"`
	ConfidenceScores json.RawMessage    `db:"confidence_scores" json:"confidence_scores" swaggertype:"object"`
	ParsingStatus    ParsingStatus      `db:"parsing_status" json:"parsing_status"`
	ParsingError     string             `db:"parsing_error" json:"parsing_error"`
	ParsedAt         *time.Time         `db:"parsed_at" json:"parsed_at"`
	ReviewStatus     ReviewStatus       `db:"review_status" json:"review_status"`
	ReviewedBy       *uuid.UUID         `db:"reviewed_by" json:"reviewed_by"`
	ReviewedAt       *time.Time         `db:"reviewed_at" json:"reviewed_at"`
	ReviewerNotes    string             `db:"reviewer_notes" json:"reviewer_notes"`
	ValidationStatus      ValidationStatus     `db:"validation_status" json:"validation_status"`
	ValidationResults     json.RawMessage      `db:"validation_results" json:"validation_results" swaggertype:"object"`
	ReconciliationStatus  ReconciliationStatus `db:"reconciliation_status" json:"reconciliation_status"`
	ParseMode             ParseMode            `db:"parse_mode" json:"parse_mode"`
	FieldProvenance       json.RawMessage      `db:"field_provenance" json:"field_provenance" swaggertype:"object"`
	SecondaryParserModel  string               `db:"secondary_parser_model" json:"secondary_parser_model"`
	ParseAttempts         int                  `db:"parse_attempts" json:"parse_attempts"`
	RetryAfter            *time.Time           `db:"retry_after" json:"retry_after,omitempty"`
	AssignedTo            *uuid.UUID           `db:"assigned_to" json:"assigned_to"`
	AssignedAt            *time.Time           `db:"assigned_at" json:"assigned_at,omitempty"`
	AssignedBy            *uuid.UUID           `db:"assigned_by" json:"assigned_by"`
	CreatedBy             uuid.UUID            `db:"created_by" json:"created_by"`
	CreatedAt             time.Time            `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time            `db:"updated_at" json:"updated_at"`
}

// DocumentTag represents a searchable tag on a document.
type DocumentTag struct {
	ID         uuid.UUID `db:"id" json:"id"`
	DocumentID uuid.UUID `db:"document_id" json:"document_id"`
	TenantID   uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Key        string    `db:"key" json:"key"`
	Value      string    `db:"value" json:"value"`
	Source     string    `db:"source" json:"source"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

// DocumentValidationRule represents a configurable validation rule for documents.
type DocumentValidationRule struct {
	ID                     uuid.UUID          `db:"id" json:"id"`
	TenantID               uuid.UUID          `db:"tenant_id" json:"tenant_id"`
	CollectionID           *uuid.UUID         `db:"collection_id" json:"collection_id"`
	DocumentType           string             `db:"document_type" json:"document_type"`
	RuleName               string             `db:"rule_name" json:"rule_name"`
	RuleType               ValidationRuleType `db:"rule_type" json:"rule_type"`
	RuleConfig             json.RawMessage    `db:"rule_config" json:"rule_config" swaggertype:"object"`
	Severity               ValidationSeverity `db:"severity" json:"severity"`
	IsActive               bool               `db:"is_active" json:"is_active"`
	IsBuiltin              bool               `db:"is_builtin" json:"is_builtin"`
	BuiltinRuleKey         *string            `db:"builtin_rule_key" json:"builtin_rule_key"`
	ReconciliationCritical bool               `db:"reconciliation_critical" json:"reconciliation_critical"`
	CreatedBy              uuid.UUID          `db:"created_by" json:"created_by"`
	CreatedAt              time.Time          `db:"created_at" json:"created_at"`
	UpdatedAt              time.Time          `db:"updated_at" json:"updated_at"`
}

// Stats holds aggregate counts for documents, collections, and their statuses.
type Stats struct {
	TotalDocuments   int `db:"total_documents" json:"total_documents"`
	TotalCollections int `db:"total_collections" json:"total_collections"`

	ParsingCompleted  int `db:"parsing_completed" json:"parsing_completed"`
	ParsingFailed     int `db:"parsing_failed" json:"parsing_failed"`
	ParsingProcessing int `db:"parsing_processing" json:"parsing_processing"`
	ParsingPending    int `db:"parsing_pending" json:"parsing_pending"`
	ParsingQueued     int `db:"parsing_queued" json:"parsing_queued"`

	ValidationValid   int `db:"validation_valid" json:"validation_valid"`
	ValidationWarning int `db:"validation_warning" json:"validation_warning"`
	ValidationInvalid int `db:"validation_invalid" json:"validation_invalid"`

	ReconciliationValid   int `db:"reconciliation_valid" json:"reconciliation_valid"`
	ReconciliationWarning int `db:"reconciliation_warning" json:"reconciliation_warning"`
	ReconciliationInvalid int `db:"reconciliation_invalid" json:"reconciliation_invalid"`

	ReviewPending  int `db:"review_pending" json:"review_pending"`
	ReviewApproved int `db:"review_approved" json:"review_approved"`
	ReviewRejected int `db:"review_rejected" json:"review_rejected"`
}

// DocumentSummary is a denormalized view of a parsed document for reporting.
type DocumentSummary struct {
	DocumentID           uuid.UUID            `db:"document_id" json:"document_id"`
	TenantID             uuid.UUID            `db:"tenant_id" json:"tenant_id"`
	CollectionID         uuid.UUID            `db:"collection_id" json:"collection_id"`
	InvoiceNumber        string               `db:"invoice_number" json:"invoice_number"`
	InvoiceDate          *time.Time           `db:"invoice_date" json:"invoice_date"`
	DueDate              *time.Time           `db:"due_date" json:"due_date"`
	InvoiceType          string               `db:"invoice_type" json:"invoice_type"`
	Currency             string               `db:"currency" json:"currency"`
	PlaceOfSupply        string               `db:"place_of_supply" json:"place_of_supply"`
	ReverseCharge        bool                 `db:"reverse_charge" json:"reverse_charge"`
	HasIRN               bool                 `db:"has_irn" json:"has_irn"`
	SellerName           string               `db:"seller_name" json:"seller_name"`
	SellerGSTIN          string               `db:"seller_gstin" json:"seller_gstin"`
	SellerState          string               `db:"seller_state" json:"seller_state"`
	SellerStateCode      string               `db:"seller_state_code" json:"seller_state_code"`
	BuyerName            string               `db:"buyer_name" json:"buyer_name"`
	BuyerGSTIN           string               `db:"buyer_gstin" json:"buyer_gstin"`
	BuyerState           string               `db:"buyer_state" json:"buyer_state"`
	BuyerStateCode       string               `db:"buyer_state_code" json:"buyer_state_code"`
	Subtotal             float64              `db:"subtotal" json:"subtotal"`
	TotalDiscount        float64              `db:"total_discount" json:"total_discount"`
	TaxableAmount        float64              `db:"taxable_amount" json:"taxable_amount"`
	CGST                 float64              `db:"cgst" json:"cgst"`
	SGST                 float64              `db:"sgst" json:"sgst"`
	IGST                 float64              `db:"igst" json:"igst"`
	Cess                 float64              `db:"cess" json:"cess"`
	TotalAmount          float64              `db:"total_amount" json:"total_amount"`
	LineItemCount        int                  `db:"line_item_count" json:"line_item_count"`
	DistinctHSNCodes     pq.StringArray       `db:"distinct_hsn_codes" json:"distinct_hsn_codes"`
	ParsingStatus        ParsingStatus        `db:"parsing_status" json:"parsing_status"`
	ReviewStatus         ReviewStatus         `db:"review_status" json:"review_status"`
	ValidationStatus     ValidationStatus     `db:"validation_status" json:"validation_status"`
	ReconciliationStatus ReconciliationStatus `db:"reconciliation_status" json:"reconciliation_status"`
	CreatedAt            time.Time            `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time            `db:"updated_at" json:"updated_at"`
}

// DocumentLineItem is a denormalized line item for fast HSN aggregation queries.
type DocumentLineItem struct {
	ID            uuid.UUID `db:"id" json:"id"`
	DocumentID    uuid.UUID `db:"document_id" json:"document_id"`
	TenantID      uuid.UUID `db:"tenant_id" json:"tenant_id"`
	ItemIndex     int       `db:"item_index" json:"item_index"`
	HSNSACCode    string    `db:"hsn_sac_code" json:"hsn_sac_code"`
	Description   string    `db:"description" json:"description"`
	Quantity      float64   `db:"quantity" json:"quantity"`
	UnitPrice     float64   `db:"unit_price" json:"unit_price"`
	Discount      float64   `db:"discount" json:"discount"`
	TaxableAmount float64   `db:"taxable_amount" json:"taxable_amount"`
	CGSTRate      float64   `db:"cgst_rate" json:"cgst_rate"`
	CGSTAmount    float64   `db:"cgst_amount" json:"cgst_amount"`
	SGSTRate      float64   `db:"sgst_rate" json:"sgst_rate"`
	SGSTAmount    float64   `db:"sgst_amount" json:"sgst_amount"`
	IGSTRate      float64   `db:"igst_rate" json:"igst_rate"`
	IGSTAmount    float64   `db:"igst_amount" json:"igst_amount"`
	Total         float64   `db:"total" json:"total"`
}

// ReportFilters holds common filter parameters for report queries.
type ReportFilters struct {
	From         *time.Time
	To           *time.Time
	CollectionID *uuid.UUID
	SellerGSTIN  string
	BuyerGSTIN   string
	Granularity  string // daily, weekly, monthly, quarterly, yearly
	UserID       uuid.UUID
	UserRole     UserRole
	Offset       int
	Limit        int
}

// SellerSummaryRow is one row in the seller summary report.
type SellerSummaryRow struct {
	SellerGSTIN         string     `db:"seller_gstin" json:"seller_gstin"`
	SellerName          string     `db:"seller_name" json:"seller_name"`
	SellerState         string     `db:"seller_state" json:"seller_state"`
	InvoiceCount        int        `db:"invoice_count" json:"invoice_count"`
	TotalAmount         float64    `db:"total_amount" json:"total_amount"`
	TotalTax            float64    `db:"total_tax" json:"total_tax"`
	CGST                float64    `db:"cgst" json:"cgst"`
	SGST                float64    `db:"sgst" json:"sgst"`
	IGST                float64    `db:"igst" json:"igst"`
	AverageInvoiceValue float64    `db:"average_invoice_value" json:"average_invoice_value"`
	FirstInvoiceDate    *time.Time `db:"first_invoice_date" json:"first_invoice_date"`
	LastInvoiceDate     *time.Time `db:"last_invoice_date" json:"last_invoice_date"`
}

// BuyerSummaryRow is one row in the buyer summary report.
type BuyerSummaryRow struct {
	BuyerGSTIN          string     `db:"buyer_gstin" json:"buyer_gstin"`
	BuyerName           string     `db:"buyer_name" json:"buyer_name"`
	BuyerState          string     `db:"buyer_state" json:"buyer_state"`
	InvoiceCount        int        `db:"invoice_count" json:"invoice_count"`
	TotalAmount         float64    `db:"total_amount" json:"total_amount"`
	TotalTax            float64    `db:"total_tax" json:"total_tax"`
	CGST                float64    `db:"cgst" json:"cgst"`
	SGST                float64    `db:"sgst" json:"sgst"`
	IGST                float64    `db:"igst" json:"igst"`
	AverageInvoiceValue float64    `db:"average_invoice_value" json:"average_invoice_value"`
	FirstInvoiceDate    *time.Time `db:"first_invoice_date" json:"first_invoice_date"`
	LastInvoiceDate     *time.Time `db:"last_invoice_date" json:"last_invoice_date"`
}

// PartyLedgerRow is one row in the party ledger report.
type PartyLedgerRow struct {
	DocumentID        uuid.UUID        `db:"document_id" json:"document_id"`
	InvoiceNumber     string           `db:"invoice_number" json:"invoice_number"`
	InvoiceDate       *time.Time       `db:"invoice_date" json:"invoice_date"`
	InvoiceType       string           `db:"invoice_type" json:"invoice_type"`
	CounterpartyName  string           `db:"counterparty_name" json:"counterparty_name"`
	CounterpartyGSTIN string           `db:"counterparty_gstin" json:"counterparty_gstin"`
	Role              string           `db:"role" json:"role"`
	Subtotal          float64          `db:"subtotal" json:"subtotal"`
	TaxableAmount     float64          `db:"taxable_amount" json:"taxable_amount"`
	CGST              float64          `db:"cgst" json:"cgst"`
	SGST              float64          `db:"sgst" json:"sgst"`
	IGST              float64          `db:"igst" json:"igst"`
	TotalAmount       float64          `db:"total_amount" json:"total_amount"`
	ValidationStatus  ValidationStatus `db:"validation_status" json:"validation_status"`
	ReviewStatus      ReviewStatus     `db:"review_status" json:"review_status"`
}

// FinancialSummaryRow is one row in the financial summary report (one per time period).
type FinancialSummaryRow struct {
	Period        string    `json:"period"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
	InvoiceCount  int       `db:"invoice_count" json:"invoice_count"`
	Subtotal      float64   `db:"subtotal" json:"subtotal"`
	TaxableAmount float64   `db:"taxable_amount" json:"taxable_amount"`
	CGST          float64   `db:"cgst" json:"cgst"`
	SGST          float64   `db:"sgst" json:"sgst"`
	IGST          float64   `db:"igst" json:"igst"`
	Cess          float64   `db:"cess" json:"cess"`
	TotalAmount   float64   `db:"total_amount" json:"total_amount"`
}

// TaxSummaryRow is one row in the tax summary report (one per time period).
type TaxSummaryRow struct {
	Period            string    `json:"period"`
	PeriodStart       time.Time `json:"period_start"`
	PeriodEnd         time.Time `json:"period_end"`
	IntrastateCount   int       `db:"intrastate_count" json:"intrastate_count"`
	IntrastateTaxable float64   `db:"intrastate_taxable" json:"intrastate_taxable"`
	CGST              float64   `db:"cgst" json:"cgst"`
	SGST              float64   `db:"sgst" json:"sgst"`
	InterstateCount   int       `db:"interstate_count" json:"interstate_count"`
	InterstateTaxable float64   `db:"interstate_taxable" json:"interstate_taxable"`
	IGST              float64   `db:"igst" json:"igst"`
	Cess              float64   `db:"cess" json:"cess"`
	TotalTax          float64   `db:"total_tax" json:"total_tax"`
}

// HSNSummaryRow is one row in the HSN summary report.
type HSNSummaryRow struct {
	HSNCode       string  `db:"hsn_code" json:"hsn_code"`
	Description   string  `db:"description" json:"description"`
	InvoiceCount  int     `db:"invoice_count" json:"invoice_count"`
	LineItemCount int     `db:"line_item_count" json:"line_item_count"`
	TotalQuantity float64 `db:"total_quantity" json:"total_quantity"`
	TaxableAmount float64 `db:"taxable_amount" json:"taxable_amount"`
	CGST          float64 `db:"cgst" json:"cgst"`
	SGST          float64 `db:"sgst" json:"sgst"`
	IGST          float64 `db:"igst" json:"igst"`
	TotalTax      float64 `db:"total_tax" json:"total_tax"`
}

// CollectionOverviewRow is one row in the collections overview report.
type CollectionOverviewRow struct {
	CollectionID         uuid.UUID `db:"collection_id" json:"collection_id"`
	CollectionName       string    `db:"collection_name" json:"collection_name"`
	DocumentCount        int       `db:"document_count" json:"document_count"`
	TotalAmount          float64   `db:"total_amount" json:"total_amount"`
	ValidationValidPct   float64   `db:"validation_valid_pct" json:"validation_valid_pct"`
	ValidationWarningPct float64   `db:"validation_warning_pct" json:"validation_warning_pct"`
	ValidationInvalidPct float64   `db:"validation_invalid_pct" json:"validation_invalid_pct"`
	ReviewApprovedPct    float64   `db:"review_approved_pct" json:"review_approved_pct"`
	ReviewPendingPct     float64   `db:"review_pending_pct" json:"review_pending_pct"`
}

// SummaryStatusUpdate holds status fields to update on document_summaries.
type SummaryStatusUpdate struct {
	ParsingStatus        ParsingStatus
	ReviewStatus         ReviewStatus
	ValidationStatus     ValidationStatus
	ReconciliationStatus ReconciliationStatus
}

// DocumentAuditEntry represents an append-only audit log entry for document mutations.
type DocumentAuditEntry struct {
	ID         uuid.UUID        `db:"id" json:"id"`
	TenantID   uuid.UUID        `db:"tenant_id" json:"tenant_id"`
	DocumentID uuid.UUID        `db:"document_id" json:"document_id"`
	UserID     *uuid.UUID       `db:"user_id" json:"user_id,omitempty"`
	Action     string           `db:"action" json:"action"`
	Changes    json.RawMessage  `db:"changes" json:"changes"`
	CreatedAt  time.Time        `db:"created_at" json:"created_at"`
}

// ServiceAccount represents a non-human API identity for programmatic access.
type ServiceAccount struct {
	ID           uuid.UUID `db:"id" json:"id"`
	TenantID     uuid.UUID `db:"tenant_id" json:"tenant_id"`
	Name         string    `db:"name" json:"name"`
	Description  string    `db:"description" json:"description"`
	APIKeyPrefix string    `db:"api_key_prefix" json:"api_key_prefix"` // first 8 chars of the key, for identification
	APIKeyHash   string    `db:"api_key_hash" json:"-"`
	IsActive     bool      `db:"is_active" json:"is_active"`
	CreatedBy    uuid.UUID `db:"created_by" json:"created_by"`
	LastUsedAt   *time.Time `db:"last_used_at" json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
}

// ServiceAccountPermission represents a service account's permission on a collection.
type ServiceAccountPermission struct {
	ID               uuid.UUID            `db:"id" json:"id"`
	ServiceAccountID uuid.UUID            `db:"service_account_id" json:"service_account_id"`
	CollectionID     uuid.UUID            `db:"collection_id" json:"collection_id"`
	TenantID         uuid.UUID            `db:"tenant_id" json:"tenant_id"`
	Permission       CollectionPermission `db:"permission" json:"permission"`
	GrantedBy        uuid.UUID            `db:"granted_by" json:"granted_by"`
}

// FileMeta stores metadata about an uploaded file.
type FileMeta struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	TenantID     uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	UploadedBy   uuid.UUID  `db:"uploaded_by" json:"uploaded_by"`
	FileName     string     `db:"file_name" json:"file_name"`
	OriginalName string     `db:"original_name" json:"original_name"`
	FileType     FileType   `db:"file_type" json:"file_type"`
	FileSize     int64      `db:"file_size" json:"file_size"`
	S3Bucket     string     `db:"s3_bucket" json:"s3_bucket"`
	S3Key        string     `db:"s3_key" json:"s3_key"`
	ContentType  string     `db:"content_type" json:"content_type"`
	Status       FileStatus `db:"status" json:"status"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
}
