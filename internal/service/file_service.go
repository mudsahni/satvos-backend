package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/port"
)

// FileUploadInput is the DTO for file upload requests.
type FileUploadInput struct {
	TenantID   uuid.UUID
	UploadedBy uuid.UUID
	File       multipart.File
	Header     *multipart.FileHeader
}

// FileService defines the file management contract.
type FileService interface {
	Upload(ctx context.Context, input FileUploadInput) (*domain.FileMeta, error)
	GetByID(ctx context.Context, tenantID, fileID uuid.UUID) (*domain.FileMeta, error)
	GetByIDForUser(ctx context.Context, tenantID, fileID, userID uuid.UUID, role domain.UserRole) (*domain.FileMeta, error)
	List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error)
	ListByUploader(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error)
	GetDownloadURL(ctx context.Context, tenantID, fileID uuid.UUID) (string, error)
	Delete(ctx context.Context, tenantID, fileID uuid.UUID) error
}

type fileService struct {
	fileRepo port.FileMetaRepository
	storage  port.ObjectStorage
	cfg      *config.S3Config
}

// NewFileService creates a new FileService implementation.
func NewFileService(
	fileRepo port.FileMetaRepository,
	storage port.ObjectStorage,
	cfg *config.S3Config,
) FileService {
	return &fileService{
		fileRepo: fileRepo,
		storage:  storage,
		cfg:      cfg,
	}
}

func (s *fileService) Upload(ctx context.Context, input FileUploadInput) (*domain.FileMeta, error) {
	// Validate file extension
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(input.Header.Filename), "."))
	fileType, ok := domain.AllowedExtensions[ext]
	if !ok {
		return nil, domain.ErrUnsupportedFileType
	}

	// Validate file size
	maxBytes := s.cfg.MaxFileSizeMB * 1024 * 1024
	if input.Header.Size > maxBytes {
		return nil, domain.ErrFileTooLarge
	}

	// Read first 512 bytes for magic-byte content type detection
	buf := make([]byte, 512)
	n, err := input.File.Read(buf)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading file header: %w", err)
	}
	detectedType := http.DetectContentType(buf[:n])

	// Validate detected content type
	_, validContent := domain.AllowedContentTypes[detectedType]
	if !validContent {
		return nil, domain.ErrUnsupportedFileType
	}

	// Seek back to beginning for upload
	if _, err := input.File.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seeking file: %w", err)
	}

	// Generate storage key and file metadata
	fileID := uuid.New()
	s3Key := fmt.Sprintf("tenants/%s/files/%s/%s", input.TenantID, fileID, input.Header.Filename)
	contentType := domain.AllowedFileTypes[fileType]

	meta := &domain.FileMeta{
		ID:           fileID,
		TenantID:     input.TenantID,
		UploadedBy:   input.UploadedBy,
		FileName:     fileID.String() + "." + ext,
		OriginalName: input.Header.Filename,
		FileType:     fileType,
		FileSize:     input.Header.Size,
		S3Bucket:     s.cfg.Bucket,
		S3Key:        s3Key,
		ContentType:  contentType,
		Status:       domain.FileStatusPending,
	}

	log.Printf("fileService.Upload: uploading file %s (%s, %d bytes) for tenant %s by user %s",
		input.Header.Filename, contentType, input.Header.Size, input.TenantID, input.UploadedBy)

	// Persist metadata with pending status
	if err := s.fileRepo.Create(ctx, meta); err != nil {
		log.Printf("fileService.Upload: failed to create file metadata: %v", err)
		return nil, fmt.Errorf("creating file metadata: %w", err)
	}

	// Upload to S3
	_, err = s.storage.Upload(ctx, port.UploadInput{
		Bucket:      s.cfg.Bucket,
		Key:         s3Key,
		Body:        input.File,
		ContentType: contentType,
		Size:        input.Header.Size,
	})
	if err != nil {
		log.Printf("fileService.Upload: S3 upload failed for file %s: %v", meta.ID, err)
		// Mark as failed
		_ = s.fileRepo.UpdateStatus(ctx, meta.TenantID, meta.ID, domain.FileStatusFailed)
		return nil, domain.ErrUploadFailed
	}

	// Mark as uploaded
	if err := s.fileRepo.UpdateStatus(ctx, meta.TenantID, meta.ID, domain.FileStatusUploaded); err != nil {
		return nil, fmt.Errorf("updating file status: %w", err)
	}
	meta.Status = domain.FileStatusUploaded

	return meta, nil
}

func (s *fileService) GetByID(ctx context.Context, tenantID, fileID uuid.UUID) (*domain.FileMeta, error) {
	return s.fileRepo.GetByID(ctx, tenantID, fileID)
}

// GetByIDForUser returns a file if the user has access. Free-tier and service account users can only access their own files.
func (s *fileService) GetByIDForUser(ctx context.Context, tenantID, fileID, userID uuid.UUID, role domain.UserRole) (*domain.FileMeta, error) {
	meta, err := s.fileRepo.GetByID(ctx, tenantID, fileID)
	if err != nil {
		return nil, err
	}
	if (role == domain.RoleFree || role == domain.RoleService) && meta.UploadedBy != userID {
		return nil, domain.ErrNotFound
	}
	return meta, nil
}

func (s *fileService) List(ctx context.Context, tenantID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error) {
	return s.fileRepo.ListByTenant(ctx, tenantID, offset, limit)
}

func (s *fileService) ListByUploader(ctx context.Context, tenantID, userID uuid.UUID, offset, limit int) ([]domain.FileMeta, int, error) {
	return s.fileRepo.ListByUploader(ctx, tenantID, userID, offset, limit)
}

func (s *fileService) GetDownloadURL(ctx context.Context, tenantID, fileID uuid.UUID) (string, error) {
	meta, err := s.fileRepo.GetByID(ctx, tenantID, fileID)
	if err != nil {
		return "", err
	}
	return s.storage.GetPresignedURL(ctx, meta.S3Bucket, meta.S3Key, s.cfg.PresignExpiry)
}

func (s *fileService) Delete(ctx context.Context, tenantID, fileID uuid.UUID) error {
	log.Printf("fileService.Delete: deleting file %s for tenant %s", fileID, tenantID)

	meta, err := s.fileRepo.GetByID(ctx, tenantID, fileID)
	if err != nil {
		return err
	}

	if err := s.storage.Delete(ctx, meta.S3Bucket, meta.S3Key); err != nil {
		log.Printf("fileService.Delete: failed to delete from S3: %v", err)
		return fmt.Errorf("deleting from storage: %w", err)
	}

	return s.fileRepo.Delete(ctx, tenantID, fileID)
}
