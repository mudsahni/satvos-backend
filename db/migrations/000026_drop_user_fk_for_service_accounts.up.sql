-- Drop FK constraints on created_by/uploaded_by/added_by columns so that
-- service accounts (whose IDs live in service_accounts, not users) can
-- create collections, upload files, and create documents.
--
-- The columns remain NOT NULL UUID — we just remove the REFERENCES.

BEGIN;

-- collections.created_by
ALTER TABLE collections DROP CONSTRAINT IF EXISTS collections_created_by_fkey;

-- file_metadata.uploaded_by
ALTER TABLE file_metadata DROP CONSTRAINT IF EXISTS file_metadata_uploaded_by_fkey;

-- collection_files.added_by
ALTER TABLE collection_files DROP CONSTRAINT IF EXISTS collection_files_added_by_fkey;

-- documents.created_by
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_created_by_fkey;

COMMIT;
