ALTER TABLE sync_events DROP COLUMN IF EXISTS auto_created_masters;

DROP INDEX IF EXISTS idx_documents_sync_review;

ALTER TABLE documents
  DROP COLUMN IF EXISTS voucher_overrides,
  DROP COLUMN IF EXISTS sync_approved_at,
  DROP COLUMN IF EXISTS sync_approved_by,
  DROP COLUMN IF EXISTS sync_review_status;
