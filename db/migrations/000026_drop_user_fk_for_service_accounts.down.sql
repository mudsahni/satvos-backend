-- Re-add FK constraints on created_by/uploaded_by/added_by columns.
-- WARNING: This will fail if any rows reference service_account IDs
-- that don't exist in the users table.

BEGIN;

ALTER TABLE collections
    ADD CONSTRAINT collections_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE file_metadata
    ADD CONSTRAINT file_metadata_uploaded_by_fkey
    FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE collection_files
    ADD CONSTRAINT collection_files_added_by_fkey
    FOREIGN KEY (added_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE documents
    ADD CONSTRAINT documents_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

COMMIT;
