ALTER TABLE documents
    ADD COLUMN sync_status VARCHAR(20) NOT NULL DEFAULT 'not_synced';

-- Backfill: set sync_status based on existing sync_events
UPDATE documents SET sync_status = 'synced'
WHERE id IN (
    SELECT DISTINCT document_id FROM sync_events
    WHERE direction = 'outbound' AND status = 'success' AND document_id IS NOT NULL
);

UPDATE documents SET sync_status = 'failed'
WHERE sync_status = 'not_synced'
AND id IN (
    SELECT DISTINCT document_id FROM sync_events
    WHERE direction = 'outbound' AND status = 'failed' AND document_id IS NOT NULL
);

UPDATE documents SET sync_status = 'pending'
WHERE sync_status = 'not_synced'
AND id IN (
    SELECT DISTINCT document_id FROM sync_events
    WHERE direction = 'outbound' AND status = 'pending' AND document_id IS NOT NULL
);
