package domain

// FileType represents the allowed file types for upload.
type FileType string

const (
	FileTypePDF FileType = "pdf"
	FileTypeJPG FileType = "jpg"
	FileTypePNG FileType = "png"
)

// AllowedFileTypes maps FileType to its MIME content type.
var AllowedFileTypes = map[FileType]string{
	FileTypePDF: "application/pdf",
	FileTypeJPG: "image/jpeg",
	FileTypePNG: "image/png",
}

// AllowedContentTypes maps MIME content types back to FileType.
var AllowedContentTypes = map[string]FileType{
	"application/pdf": FileTypePDF,
	"image/jpeg":      FileTypeJPG,
	"image/png":       FileTypePNG,
}

// AllowedExtensions maps file extensions (without dot) to FileType.
var AllowedExtensions = map[string]FileType{
	"pdf":  FileTypePDF,
	"jpg":  FileTypeJPG,
	"jpeg": FileTypeJPG,
	"png":  FileTypePNG,
}

// UserRole defines the role hierarchy within a tenant.
type UserRole string

const (
	RoleAdmin   UserRole = "admin"
	RoleManager UserRole = "manager"
	RoleMember  UserRole = "member"
	RoleViewer  UserRole = "viewer"
	RoleFree    UserRole = "free"
	RoleService UserRole = "service"
)

// ValidUserRoles maps valid role strings for validation.
var ValidUserRoles = map[UserRole]bool{
	RoleAdmin:   true,
	RoleManager: true,
	RoleMember:  true,
	RoleViewer:  true,
	RoleFree:    true,
	RoleService: true,
}

// RoleLevel returns the numeric level for role comparison.
// Higher value = more access.
func RoleLevel(role UserRole) int {
	switch role {
	case RoleAdmin:
		return 4
	case RoleManager:
		return 3
	case RoleMember:
		return 2
	case RoleViewer:
		return 1
	case RoleFree:
		return 0
	case RoleService:
		return 0
	default:
		return 0
	}
}

// ImplicitCollectionPerm returns the implicit collection permission for a tenant role.
// admin → owner, manager → editor, member → viewer, viewer → "" (none), free → "" (none).
func ImplicitCollectionPerm(role UserRole) CollectionPermission {
	switch role {
	case RoleAdmin:
		return CollectionPermOwner
	case RoleManager:
		return CollectionPermEditor
	case RoleMember:
		return CollectionPermViewer
	default:
		return ""
	}
}

// CollectionPermission defines the access level for a collection.
type CollectionPermission string

const (
	CollectionPermOwner  CollectionPermission = "owner"
	CollectionPermEditor CollectionPermission = "editor"
	CollectionPermViewer CollectionPermission = "viewer"
)

// ValidCollectionPermissions maps valid permission strings.
var ValidCollectionPermissions = map[CollectionPermission]bool{
	CollectionPermOwner:  true,
	CollectionPermEditor: true,
	CollectionPermViewer: true,
}

// CollectionPermLevel returns the numeric level for permission comparison.
// Higher value = more access.
func CollectionPermLevel(p CollectionPermission) int {
	switch p {
	case CollectionPermOwner:
		return 3
	case CollectionPermEditor:
		return 2
	case CollectionPermViewer:
		return 1
	default:
		return 0
	}
}

// ParsingStatus represents the lifecycle of document parsing.
type ParsingStatus string

const (
	ParsingStatusPending    ParsingStatus = "pending"
	ParsingStatusProcessing ParsingStatus = "processing"
	ParsingStatusCompleted  ParsingStatus = "completed"
	ParsingStatusFailed     ParsingStatus = "failed"
	ParsingStatusQueued     ParsingStatus = "queued"
)

// ValidParsingStatuses maps valid parsing status strings for filter validation.
var ValidParsingStatuses = map[ParsingStatus]bool{
	ParsingStatusPending:    true,
	ParsingStatusProcessing: true,
	ParsingStatusCompleted:  true,
	ParsingStatusFailed:     true,
	ParsingStatusQueued:     true,
}

// ReviewStatus represents the human review state of a document.
type ReviewStatus string

const (
	ReviewStatusPending  ReviewStatus = "pending"
	ReviewStatusApproved ReviewStatus = "approved"
	ReviewStatusRejected ReviewStatus = "rejected"
)

// ValidReviewStatuses maps valid review status strings for filter validation.
var ValidReviewStatuses = map[ReviewStatus]bool{
	ReviewStatusPending:  true,
	ReviewStatusApproved: true,
	ReviewStatusRejected: true,
}

// ValidationRuleType defines the kind of validation to perform.
type ValidationRuleType string

const (
	ValidationRuleRequired   ValidationRuleType = "required_field"
	ValidationRuleRegex      ValidationRuleType = "regex"
	ValidationRuleSumCheck   ValidationRuleType = "sum_check"
	ValidationRuleCrossField ValidationRuleType = "cross_field"
	ValidationRuleCustom     ValidationRuleType = "custom"
)

// ValidationSeverity defines how critical a validation failure is.
type ValidationSeverity string

const (
	ValidationSeverityError   ValidationSeverity = "error"
	ValidationSeverityWarning ValidationSeverity = "warning"
)

// ValidationStatus represents the overall validation state of a document.
type ValidationStatus string

const (
	ValidationStatusPending ValidationStatus = "pending"
	ValidationStatusValid   ValidationStatus = "valid"
	ValidationStatusInvalid ValidationStatus = "invalid"
	ValidationStatusWarning ValidationStatus = "warning"
)

// ValidValidationStatuses maps valid validation status strings for filter validation.
var ValidValidationStatuses = map[ValidationStatus]bool{
	ValidationStatusPending: true,
	ValidationStatusValid:   true,
	ValidationStatusInvalid: true,
	ValidationStatusWarning: true,
}

// ReconciliationStatus represents the GST reconciliation readiness of a document.
type ReconciliationStatus string

const (
	ReconciliationStatusPending ReconciliationStatus = "pending"
	ReconciliationStatusValid   ReconciliationStatus = "valid"
	ReconciliationStatusInvalid ReconciliationStatus = "invalid"
	ReconciliationStatusWarning ReconciliationStatus = "warning"
)

// ValidReconciliationStatuses maps valid reconciliation status strings for filter validation.
var ValidReconciliationStatuses = map[ReconciliationStatus]bool{
	ReconciliationStatusPending: true,
	ReconciliationStatusValid:   true,
	ReconciliationStatusInvalid: true,
	ReconciliationStatusWarning: true,
}

// ParseMode defines how document parsing is performed.
type ParseMode string

const (
	ParseModeSingle ParseMode = "single"
	ParseModeDual   ParseMode = "dual"
)

// ValidParseModes maps valid parse mode strings.
var ValidParseModes = map[ParseMode]bool{
	ParseModeSingle: true,
	ParseModeDual:   true,
}

// FieldValidationStatus represents the computed validation state of a single field.
type FieldValidationStatus string

const (
	FieldStatusValid   FieldValidationStatus = "valid"
	FieldStatusInvalid FieldValidationStatus = "invalid"
	FieldStatusUnsure  FieldValidationStatus = "unsure"
)

// AuthProvider identifies the authentication provider for a user account.
type AuthProvider string

const (
	AuthProviderEmail  AuthProvider = "email"
	AuthProviderGoogle AuthProvider = "google"
	AuthProviderAPIKey AuthProvider = "api_key"
)

// AuditAction identifies the type of document mutation recorded in the audit log.
type AuditAction string

const (
	AuditDocumentCreated             AuditAction = "document.created"
	AuditDocumentParseCompleted      AuditAction = "document.parse_completed"
	AuditDocumentParseFailed         AuditAction = "document.parse_failed"
	AuditDocumentParseQueued         AuditAction = "document.parse_queued"
	AuditDocumentRetry               AuditAction = "document.retry"
	AuditDocumentReview              AuditAction = "document.review"
	AuditDocumentEditStructured      AuditAction = "document.edit_structured_data"
	AuditDocumentValidate            AuditAction = "document.validate"
	AuditDocumentValidationCompleted AuditAction = "document.validation_completed"
	AuditDocumentTagsAdded           AuditAction = "document.tags_added"
	AuditDocumentTagDeleted          AuditAction = "document.tag_deleted"
	AuditDocumentDeleted             AuditAction = "document.deleted"
	AuditDocumentAssigned            AuditAction = "document.assigned"
)

// FileStatus represents the lifecycle of an uploaded file.
type FileStatus string

const (
	FileStatusPending  FileStatus = "pending"
	FileStatusUploaded FileStatus = "uploaded"
	FileStatusFailed   FileStatus = "failed"
	FileStatusDeleted  FileStatus = "deleted"
)

// AgentStatus represents the lifecycle status of a connector agent.
type AgentStatus string

const (
	AgentStatusRegistered   AgentStatus = "registered"
	AgentStatusOnline       AgentStatus = "online"
	AgentStatusOffline      AgentStatus = "offline"
	AgentStatusDisconnected AgentStatus = "disconnected"
)

// ValidAgentStatuses maps valid agent status strings for validation.
var ValidAgentStatuses = map[AgentStatus]bool{
	AgentStatusRegistered:   true,
	AgentStatusOnline:       true,
	AgentStatusOffline:      true,
	AgentStatusDisconnected: true,
}

// DocumentSyncStatus represents the Tally sync status of a document.
type DocumentSyncStatus string

const (
	DocumentSyncStatusNone    DocumentSyncStatus = "not_synced"
	DocumentSyncStatusPending DocumentSyncStatus = "pending"
	DocumentSyncStatusSynced  DocumentSyncStatus = "synced"
	DocumentSyncStatusFailed  DocumentSyncStatus = "failed"
)

// SyncDirection indicates whether a sync event is agent-to-cloud or cloud-to-agent.
type SyncDirection string

const (
	SyncDirectionInbound  SyncDirection = "inbound"
	SyncDirectionOutbound SyncDirection = "outbound"
)

// SyncStatus represents the outcome of a sync event.
type SyncStatus string

const (
	SyncStatusPending SyncStatus = "pending"
	SyncStatusSuccess SyncStatus = "success"
	SyncStatusFailed  SyncStatus = "failed"
	SyncStatusSkipped SyncStatus = "skipped"
)

// ValidSyncStatuses maps valid sync status strings for validation.
var ValidSyncStatuses = map[SyncStatus]bool{
	SyncStatusPending: true,
	SyncStatusSuccess: true,
	SyncStatusFailed:  true,
	SyncStatusSkipped: true,
}
