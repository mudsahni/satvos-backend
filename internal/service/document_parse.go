package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/parser"
	"satvos/internal/port"
	"satvos/internal/validator/invoice"
)

const defaultMaxParseAttempts = 5

func (s *documentService) CreateAndParse(ctx context.Context, input *CreateDocumentInput) (*domain.Document, error) {
	// Check editor+ permission on the collection
	if err := s.requireCollectionPerm(ctx, input.CollectionID, input.CreatedBy, input.Role, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	// Check and increment quota (no-op for unlimited users)
	if err := s.userRepo.CheckAndIncrementQuota(ctx, input.TenantID, input.CreatedBy); err != nil {
		return nil, err
	}

	// Verify the file exists
	file, err := s.fileRepo.GetByID(ctx, input.TenantID, input.FileID)
	if err != nil {
		return nil, fmt.Errorf("looking up file: %w", err)
	}

	parseMode := input.ParseMode
	if parseMode == "" {
		parseMode = domain.ParseModeSingle
	}
	// Fall back to single if dual requested but no merge parser configured
	if parseMode == domain.ParseModeDual && s.mergeParser == nil {
		log.Printf("documentService.CreateAndParse: dual parse requested but no merge parser configured, falling back to single")
		parseMode = domain.ParseModeSingle
	}

	name := input.Name
	if name == "" {
		name = file.OriginalName
	}

	doc := &domain.Document{
		ID:                   uuid.New(),
		TenantID:             input.TenantID,
		CollectionID:         input.CollectionID,
		FileID:               input.FileID,
		Name:                 name,
		DocumentType:         input.DocumentType,
		ParsingStatus:        domain.ParsingStatusPending,
		ReviewStatus:         domain.ReviewStatusPending,
		ValidationStatus:     domain.ValidationStatusPending,
		ReconciliationStatus: domain.ReconciliationStatusPending,
		ValidationResults:    json.RawMessage("[]"),
		StructuredData:       json.RawMessage("{}"),
		ConfidenceScores:     json.RawMessage("{}"),
		ParseMode:            parseMode,
		FieldProvenance:      json.RawMessage("{}"),
		CreatedBy:            input.CreatedBy,
	}

	log.Printf("documentService.CreateAndParse: creating document %s for file %s (tenant %s)",
		doc.ID, input.FileID, input.TenantID)

	if err := s.docRepo.Create(ctx, doc); err != nil {
		return nil, fmt.Errorf("creating document: %w", err)
	}

	changesJSON, _ := json.Marshal(map[string]interface{}{
		"collection_id": input.CollectionID, "file_id": input.FileID,
		"document_type": input.DocumentType, "parse_mode": string(parseMode),
	})
	s.audit(ctx, doc.TenantID, doc.ID, &input.CreatedBy, domain.AuditDocumentCreated, changesJSON)

	// Save user-provided tags
	if len(input.Tags) > 0 && s.tagRepo != nil {
		tags := make([]domain.DocumentTag, 0, len(input.Tags))
		for k, v := range input.Tags {
			tags = append(tags, domain.DocumentTag{
				ID:         uuid.New(),
				DocumentID: doc.ID,
				TenantID:   doc.TenantID,
				Key:        k,
				Value:      v,
				Source:     "user",
			})
		}
		if tagErr := s.tagRepo.CreateBatch(ctx, tags); tagErr != nil {
			log.Printf("documentService.CreateAndParse: failed to save user tags for %s: %v", doc.ID, tagErr)
		}
	}

	// Copy before launching goroutine so the caller's value is independent of background work
	result := *doc

	// Launch background parsing
	go s.parseInBackground(doc.ID, doc.TenantID)

	return &result, nil
}

func (s *documentService) selectParser(mode domain.ParseMode) port.DocumentParser {
	if mode == domain.ParseModeDual && s.mergeParser != nil {
		return s.mergeParser
	}
	return s.parser
}

func (s *documentService) parseInBackground(docID, tenantID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Printf("documentService.parseInBackground: starting parsing for document %s", docID)

	// Set status to processing
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		log.Printf("documentService.parseInBackground: failed to get document %s: %v", docID, err)
		return
	}
	doc.ParseAttempts++
	doc.ParsingStatus = domain.ParsingStatusProcessing
	if err := s.docRepo.UpdateStructuredData(ctx, doc); err != nil {
		log.Printf("documentService.parseInBackground: failed to set processing status for %s: %v", docID, err)
		return
	}

	s.ParseDocument(ctx, doc, defaultMaxParseAttempts)
}

// ParseDocument performs the core parse logic: file lookup, S3 download, LLM parse,
// error handling (with rate-limit queueing), result saving, auto-tags, and validation.
// It is called by both parseInBackground and the queue worker.
// The doc must already be in processing status with ParseAttempts incremented.
func (s *documentService) ParseDocument(ctx context.Context, doc *domain.Document, maxAttempts int) {
	// Look up file for S3 coordinates
	file, err := s.fileRepo.GetByID(ctx, doc.TenantID, doc.FileID)
	if err != nil {
		s.failParsing(ctx, doc, fmt.Sprintf("downloading file: %v", err))
		return
	}

	// Download file bytes from S3
	fileBytes, err := s.storage.Download(ctx, file.S3Bucket, file.S3Key)
	if err != nil {
		s.failParsing(ctx, doc, fmt.Sprintf("downloading file: %v", err))
		return
	}

	// Select parser based on document's parse mode
	activeParser := s.selectParser(doc.ParseMode)

	// Call parser
	output, err := activeParser.Parse(ctx, port.ParseInput{
		FileBytes:    fileBytes,
		ContentType:  file.ContentType,
		DocumentType: doc.DocumentType,
	})
	if err != nil {
		s.handleParseError(ctx, doc, err, maxAttempts)
		return
	}

	// Update with results
	now := time.Now().UTC()
	doc.StructuredData = output.StructuredData
	doc.ConfidenceScores = output.ConfidenceScores
	doc.ParserModel = output.ModelUsed
	doc.SecondaryParserModel = output.SecondaryModel
	doc.ParserPrompt = output.PromptUsed
	doc.ParsingStatus = domain.ParsingStatusCompleted
	doc.ParsingError = ""
	doc.ParsedAt = &now
	doc.RetryAfter = nil

	// Save field provenance if present
	if len(output.FieldProvenance) > 0 {
		if provenanceJSON, jsonErr := json.Marshal(output.FieldProvenance); jsonErr == nil {
			doc.FieldProvenance = provenanceJSON
		}
	}

	if err := s.docRepo.UpdateStructuredData(ctx, doc); err != nil {
		log.Printf("documentService.ParseDocument: failed to save results for %s: %v", doc.ID, err)
		return
	}

	parseChanges, _ := json.Marshal(map[string]interface{}{
		"parser_model": doc.ParserModel, "parse_mode": string(doc.ParseMode), "attempt": doc.ParseAttempts,
	})
	s.audit(ctx, doc.TenantID, doc.ID, nil, domain.AuditDocumentParseCompleted, parseChanges)

	log.Printf("documentService.ParseDocument: document %s parsed successfully", doc.ID)

	// Unmarshal structured data once for tags + summary
	var inv invoice.GSTInvoice
	if err := json.Unmarshal(doc.StructuredData, &inv); err != nil {
		log.Printf("documentService.ParseDocument: failed to unmarshal structured_data for %s: %v", doc.ID, err)
	} else {
		if s.tagRepo != nil {
			s.extractAndSaveAutoTags(ctx, doc.ID, doc.TenantID, &inv)
		}
		s.upsertSummary(ctx, doc, &inv)
	}

	// Run validation after successful parsing
	if s.validator != nil {
		if err := s.validator.ValidateDocument(ctx, doc.TenantID, doc.ID); err != nil {
			log.Printf("documentService.ParseDocument: validation failed for %s: %v", doc.ID, err)
		} else {
			s.auditValidationCompleted(ctx, doc.TenantID, doc.ID, nil, "parse")
			// Update summary statuses after successful validation
			if validatedDoc, fetchErr := s.docRepo.GetByID(ctx, doc.TenantID, doc.ID); fetchErr == nil {
				s.updateSummaryStatuses(ctx, validatedDoc)
			}
		}
	}
}

// handleParseError checks if the error is a rate limit and queues the document for retry
// if under the max attempts threshold. Otherwise, marks parsing as permanently failed.
func (s *documentService) handleParseError(ctx context.Context, doc *domain.Document, parseErr error, maxAttempts int) {
	var rlErr *parser.RateLimitError
	if errors.As(parseErr, &rlErr) && doc.ParseAttempts < maxAttempts {
		retryAt := time.Now().Add(rlErr.RetryAfter)
		doc.ParsingStatus = domain.ParsingStatusQueued
		doc.ParsingError = fmt.Sprintf("rate limited by %s, queued for retry", rlErr.Provider)
		doc.RetryAfter = &retryAt
		if err := s.docRepo.UpdateStructuredData(ctx, doc); err != nil {
			log.Printf("documentService.handleParseError: failed to queue document %s: %v", doc.ID, err)
		} else {
			queueChanges, _ := json.Marshal(map[string]interface{}{
				"retry_after": retryAt.Format(time.RFC3339), "attempt": doc.ParseAttempts,
			})
			s.audit(ctx, doc.TenantID, doc.ID, nil, domain.AuditDocumentParseQueued, queueChanges)
			log.Printf("documentService.handleParseError: document %s queued for retry after %s", doc.ID, retryAt.Format(time.RFC3339))
		}
		return
	}
	s.failParsing(ctx, doc, fmt.Sprintf("parsing document: %v", parseErr))
}

func (s *documentService) failParsing(ctx context.Context, doc *domain.Document, errMsg string) {
	log.Printf("documentService.failParsing: document %s failed: %s", doc.ID, errMsg)
	doc.ParsingStatus = domain.ParsingStatusFailed
	doc.ParsingError = errMsg
	doc.RetryAfter = nil
	if err := s.docRepo.UpdateStructuredData(ctx, doc); err != nil {
		log.Printf("documentService.failParsing: failed to update status for %s: %v", doc.ID, err)
	}
	failChanges, _ := json.Marshal(map[string]interface{}{"error": errMsg, "attempt": doc.ParseAttempts})
	s.audit(ctx, doc.TenantID, doc.ID, nil, domain.AuditDocumentParseFailed, failChanges)
}

func (s *documentService) RetryParse(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) (*domain.Document, error) {
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}

	// Check editor+ permission on the collection
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	// Verify the file still exists
	if _, err := s.fileRepo.GetByID(ctx, tenantID, doc.FileID); err != nil {
		return nil, fmt.Errorf("looking up file for retry: %w", err)
	}

	// Delete auto-tags before re-parsing
	if s.tagRepo != nil {
		if tagErr := s.tagRepo.DeleteByDocumentAndSource(ctx, docID, "auto"); tagErr != nil {
			log.Printf("documentService.RetryParse: failed to delete auto-tags for %s: %v", docID, tagErr)
		}
	}

	// Reset to pending and clear assignment
	doc.ParsingStatus = domain.ParsingStatusPending
	doc.ParsingError = ""
	doc.StructuredData = json.RawMessage("{}")
	doc.ConfidenceScores = json.RawMessage("{}")
	doc.AssignedTo = nil
	doc.AssignedAt = nil
	doc.AssignedBy = nil
	if err := s.docRepo.UpdateStructuredData(ctx, doc); err != nil {
		return nil, fmt.Errorf("resetting document for retry: %w", err)
	}

	s.audit(ctx, tenantID, docID, &userID, domain.AuditDocumentRetry, nil)

	log.Printf("documentService.RetryParse: retrying parsing for document %s", docID)

	// Copy before launching goroutine so the caller's value is independent of background work
	result := *doc

	go s.parseInBackground(doc.ID, doc.TenantID)

	return &result, nil
}
