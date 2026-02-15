CREATE TABLE IF NOT EXISTS document_line_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    item_index  INT NOT NULL,
    hsn_sac_code VARCHAR(20) NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    quantity     NUMERIC(18,4) NOT NULL DEFAULT 0,
    unit_price   NUMERIC(18,4) NOT NULL DEFAULT 0,
    discount     NUMERIC(18,4) NOT NULL DEFAULT 0,
    taxable_amount NUMERIC(18,4) NOT NULL DEFAULT 0,
    cgst_rate    NUMERIC(8,4) NOT NULL DEFAULT 0,
    cgst_amount  NUMERIC(18,4) NOT NULL DEFAULT 0,
    sgst_rate    NUMERIC(8,4) NOT NULL DEFAULT 0,
    sgst_amount  NUMERIC(18,4) NOT NULL DEFAULT 0,
    igst_rate    NUMERIC(8,4) NOT NULL DEFAULT 0,
    igst_amount  NUMERIC(18,4) NOT NULL DEFAULT 0,
    total        NUMERIC(18,4) NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_document_line_items_document ON document_line_items (document_id);
CREATE INDEX idx_document_line_items_tenant ON document_line_items (tenant_id);
CREATE INDEX idx_document_line_items_hsn ON document_line_items (hsn_sac_code) WHERE hsn_sac_code != '';
