-- New columns on documents table for sync review workflow
ALTER TABLE documents
  ADD COLUMN sync_review_status TEXT NOT NULL DEFAULT 'not_applicable',
  ADD COLUMN sync_approved_by UUID,
  ADD COLUMN sync_approved_at TIMESTAMPTZ,
  ADD COLUMN voucher_overrides JSONB DEFAULT NULL;

-- Backfill: already-approved docs that have been synced → 'approved'
UPDATE documents SET sync_review_status = 'approved'
  WHERE review_status = 'approved' AND sync_status IN ('synced', 'pending');

-- Backfill: approved but not yet synced → 'pending' (enter queue)
UPDATE documents SET sync_review_status = 'pending'
  WHERE review_status = 'approved' AND sync_status = 'not_synced';

-- Index for queue queries
CREATE INDEX idx_documents_sync_review ON documents(tenant_id, sync_review_status)
  WHERE sync_review_status IN ('pending', 'approved');

-- Track auto-created masters on sync_events
ALTER TABLE sync_events ADD COLUMN auto_created_masters JSONB;
