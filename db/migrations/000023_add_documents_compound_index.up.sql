CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_documents_tenant_collection_created
    ON documents (tenant_id, collection_id, created_at DESC);
