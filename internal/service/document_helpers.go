package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/normalize"
	"satvos/internal/validator/invoice"
)

// audit records a document mutation in the audit log. Failures are logged but never block business logic.
func (s *documentService) audit(ctx context.Context, tenantID, docID uuid.UUID, userID *uuid.UUID, action domain.AuditAction, changes json.RawMessage) {
	if s.auditRepo == nil {
		return
	}
	if changes == nil {
		changes = json.RawMessage("{}")
	}
	entry := &domain.DocumentAuditEntry{
		ID:         uuid.New(),
		TenantID:   tenantID,
		DocumentID: docID,
		UserID:     userID,
		Action:     string(action),
		Changes:    changes,
	}
	if err := s.auditRepo.Create(ctx, entry); err != nil {
		log.Printf("documentService.audit: failed to write audit entry for %s/%s: %v", action, docID, err)
	}
}

func (s *documentService) auditValidationCompleted(ctx context.Context, tenantID, docID uuid.UUID, userID *uuid.UUID, trigger string) {
	if s.auditRepo == nil {
		return
	}
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		log.Printf("documentService.auditValidationCompleted: failed to fetch doc %s: %v", docID, err)
		return
	}
	changes, _ := json.Marshal(map[string]string{
		"validation_status":     string(doc.ValidationStatus),
		"reconciliation_status": string(doc.ReconciliationStatus),
		"trigger":               trigger,
	})
	s.audit(ctx, tenantID, docID, userID, domain.AuditDocumentValidationCompleted, changes)
}

// upsertSummary builds a DocumentSummary from a Document and upserts it.
// Non-blocking: errors are logged but never returned.
func (s *documentService) upsertSummary(ctx context.Context, doc *domain.Document, inv *invoice.GSTInvoice) {
	if s.summaryRepo == nil {
		return
	}

	// If caller didn't supply a pre-parsed invoice, unmarshal here
	if inv == nil {
		inv = new(invoice.GSTInvoice)
		if err := json.Unmarshal(doc.StructuredData, inv); err != nil {
			log.Printf("documentService.upsertSummary: failed to unmarshal structured_data for %s: %v", doc.ID, err)
			return
		}
	}

	summary := &domain.DocumentSummary{
		DocumentID:           doc.ID,
		TenantID:             doc.TenantID,
		CollectionID:         doc.CollectionID,
		InvoiceNumber:        inv.Invoice.InvoiceNumber,
		InvoiceType:          inv.Invoice.InvoiceType,
		Currency:             inv.Invoice.Currency,
		PlaceOfSupply:        inv.Invoice.PlaceOfSupply,
		ReverseCharge:        inv.Invoice.ReverseCharge,
		HasIRN:               inv.Invoice.IRN != "",
		SellerName:           inv.Seller.Name,
		SellerGSTIN:          inv.Seller.GSTIN,
		SellerState:          inv.Seller.State,
		SellerStateCode:      inv.Seller.StateCode,
		BuyerName:            inv.Buyer.Name,
		BuyerGSTIN:           inv.Buyer.GSTIN,
		BuyerState:           inv.Buyer.State,
		BuyerStateCode:       inv.Buyer.StateCode,
		Subtotal:             inv.Totals.Subtotal,
		TotalDiscount:        inv.Totals.TotalDiscount,
		TaxableAmount:        inv.Totals.TaxableAmount,
		CGST:                 inv.Totals.CGST,
		SGST:                 inv.Totals.SGST,
		IGST:                 inv.Totals.IGST,
		Cess:                 inv.Totals.Cess,
		TotalAmount:          inv.Totals.Total,
		LineItemCount:        len(inv.LineItems),
		ParsingStatus:        doc.ParsingStatus,
		ReviewStatus:         doc.ReviewStatus,
		ValidationStatus:     doc.ValidationStatus,
		ReconciliationStatus: doc.ReconciliationStatus,
	}

	// Parse invoice date
	summary.InvoiceDate = parseInvoiceDate(inv.Invoice.InvoiceDate)
	summary.DueDate = parseInvoiceDate(inv.Invoice.DueDate)

	// Collect distinct HSN codes
	hsnSet := make(map[string]struct{})
	for i := range inv.LineItems {
		if inv.LineItems[i].HSNSACCode != "" {
			hsnSet[inv.LineItems[i].HSNSACCode] = struct{}{}
		}
	}
	hsns := make([]string, 0, len(hsnSet))
	for code := range hsnSet {
		hsns = append(hsns, code)
	}
	summary.DistinctHSNCodes = hsns

	if err := s.summaryRepo.Upsert(ctx, summary); err != nil {
		log.Printf("documentService.upsertSummary: failed for %s: %v", doc.ID, err)
	}

	// Upsert denormalized line items for HSN queries
	if len(inv.LineItems) > 0 {
		items := make([]domain.DocumentLineItem, len(inv.LineItems))
		for i := range inv.LineItems {
			li := &inv.LineItems[i]
			items[i] = domain.DocumentLineItem{
				DocumentID:    doc.ID,
				TenantID:      doc.TenantID,
				ItemIndex:     i,
				HSNSACCode:    li.HSNSACCode,
				Description:   li.Description,
				Quantity:      li.Quantity,
				UnitPrice:     li.UnitPrice,
				Discount:      li.Discount,
				TaxableAmount: li.TaxableAmount,
				CGSTRate:      li.CGSTRate,
				CGSTAmount:    li.CGSTAmount,
				SGSTRate:      li.SGSTRate,
				SGSTAmount:    li.SGSTAmount,
				IGSTRate:      li.IGSTRate,
				IGSTAmount:    li.IGSTAmount,
				Total:         li.Total,
			}
		}
		if err := s.summaryRepo.ReplaceLineItems(ctx, doc.ID, doc.TenantID, items); err != nil {
			log.Printf("documentService.upsertSummary: failed to replace line items for %s: %v", doc.ID, err)
		}
	}
}

// updateSummaryStatuses updates only the status columns on the summary row.
func (s *documentService) updateSummaryStatuses(ctx context.Context, doc *domain.Document) {
	if s.summaryRepo == nil {
		return
	}
	if err := s.summaryRepo.UpdateStatuses(ctx, doc.ID, domain.SummaryStatusUpdate{
		ParsingStatus:        doc.ParsingStatus,
		ReviewStatus:         doc.ReviewStatus,
		ValidationStatus:     doc.ValidationStatus,
		ReconciliationStatus: doc.ReconciliationStatus,
	}); err != nil {
		log.Printf("documentService.updateSummaryStatuses: failed for %s: %v", doc.ID, err)
	}
}

// parseInvoiceDate attempts multiple date formats from LLM output.
func parseInvoiceDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	formats := []string{"2006-01-02", "02/01/2006", "02-01-2006", "01/02/2006", "January 2, 2006", "Jan 2, 2006", "2 January 2006"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}

func (s *documentService) ListTags(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole) ([]domain.DocumentTag, error) {
	// Verify document exists and user has access
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermViewer); err != nil {
		return nil, err
	}
	return s.tagRepo.ListByDocument(ctx, docID)
}

func (s *documentService) AddTags(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole, tagsMap map[string]string) ([]domain.DocumentTag, error) {
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return nil, err
	}

	tags := make([]domain.DocumentTag, 0, len(tagsMap))
	for k, v := range tagsMap {
		tags = append(tags, domain.DocumentTag{
			ID:         uuid.New(),
			DocumentID: docID,
			TenantID:   tenantID,
			Key:        k,
			Value:      v,
			Source:     "user",
		})
	}

	if err := s.tagRepo.CreateBatch(ctx, tags); err != nil {
		return nil, fmt.Errorf("adding tags: %w", err)
	}

	tagChanges, _ := json.Marshal(tagsMap)
	s.audit(ctx, tenantID, docID, &userID, domain.AuditDocumentTagsAdded, tagChanges)

	return tags, nil
}

func (s *documentService) DeleteTag(ctx context.Context, tenantID, docID, userID uuid.UUID, role domain.UserRole, tagID uuid.UUID) error {
	doc, err := s.docRepo.GetByID(ctx, tenantID, docID)
	if err != nil {
		return err
	}
	if err := s.requireCollectionPerm(ctx, doc.CollectionID, userID, role, domain.CollectionPermEditor); err != nil {
		return err
	}
	if err := s.tagRepo.DeleteByID(ctx, docID, tagID); err != nil {
		return err
	}
	deleteTagChanges, _ := json.Marshal(map[string]interface{}{"tag_id": tagID})
	s.audit(ctx, tenantID, docID, &userID, domain.AuditDocumentTagDeleted, deleteTagChanges)
	return nil
}

func (s *documentService) SearchByTag(ctx context.Context, tenantID uuid.UUID, key, value string, offset, limit int) ([]domain.Document, int, error) {
	return s.tagRepo.SearchByTag(ctx, tenantID, key, value, offset, limit)
}

func (s *documentService) extractAndSaveAutoTags(ctx context.Context, docID, tenantID uuid.UUID, inv *invoice.GSTInvoice) {
	tagMap := map[string]string{}
	if inv.Invoice.InvoiceNumber != "" {
		tagMap["invoice_number"] = inv.Invoice.InvoiceNumber
	}
	if inv.Invoice.InvoiceDate != "" {
		tagMap["invoice_date"] = inv.Invoice.InvoiceDate
	}
	if inv.Seller.Name != "" {
		tagMap["seller_name"] = normalize.CompanyName(inv.Seller.Name)
	}
	if inv.Seller.GSTIN != "" {
		tagMap["seller_gstin"] = inv.Seller.GSTIN
	}
	if inv.Buyer.Name != "" {
		tagMap["buyer_name"] = normalize.CompanyName(inv.Buyer.Name)
	}
	if inv.Buyer.GSTIN != "" {
		tagMap["buyer_gstin"] = inv.Buyer.GSTIN
	}
	if inv.Invoice.InvoiceType != "" {
		tagMap["invoice_type"] = inv.Invoice.InvoiceType
	}
	if inv.Invoice.PlaceOfSupply != "" {
		tagMap["place_of_supply"] = inv.Invoice.PlaceOfSupply
	}
	if inv.Invoice.IRN != "" {
		tagMap["irn"] = inv.Invoice.IRN
	}
	if inv.Totals.Total != 0 {
		tagMap["total_amount"] = fmt.Sprintf("%.2f", inv.Totals.Total)
	}

	if len(tagMap) == 0 {
		return
	}

	// Delete existing auto-tags and save new ones
	if err := s.tagRepo.DeleteByDocumentAndSource(ctx, docID, "auto"); err != nil {
		log.Printf("documentService.extractAndSaveAutoTags: failed to delete old auto-tags for %s: %v", docID, err)
	}

	tags := make([]domain.DocumentTag, 0, len(tagMap))
	for k, v := range tagMap {
		tags = append(tags, domain.DocumentTag{
			ID:         uuid.New(),
			DocumentID: docID,
			TenantID:   tenantID,
			Key:        k,
			Value:      v,
			Source:     "auto",
		})
	}

	if err := s.tagRepo.CreateBatch(ctx, tags); err != nil {
		log.Printf("documentService.extractAndSaveAutoTags: failed to save auto-tags for %s: %v", docID, err)
	}
}
