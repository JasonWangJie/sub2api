-- 企业开票：用户最近一次资料、申请快照与不可重复开票的明细关联。
CREATE TABLE IF NOT EXISTS invoice_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    tax_number VARCHAR(64) NOT NULL,
    company_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invoice_applications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    application_no VARCHAR(64) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    tax_number VARCHAR(64) NOT NULL,
    company_name VARCHAR(255) NOT NULL,
    total_amount DECIMAL(20,2) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    rejection_reason TEXT,
    completed_at TIMESTAMPTZ,
    completed_by BIGINT,
    rejected_at TIMESTAMPTZ,
    rejected_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invoice_applications_user_status
    ON invoice_applications(user_id, status);
CREATE INDEX IF NOT EXISTS idx_invoice_applications_created_at
    ON invoice_applications(created_at DESC);

CREATE TABLE IF NOT EXISTS invoice_application_items (
    id BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES invoice_applications(id) ON DELETE RESTRICT,
    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT NOT NULL,
    source_reference VARCHAR(128) NOT NULL,
    amount DECIMAL(20,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT invoice_application_items_source_unique UNIQUE (source_type, source_id)
);

CREATE INDEX IF NOT EXISTS idx_invoice_application_items_application_id
    ON invoice_application_items(application_id);

COMMENT ON TABLE invoice_applications IS '企业开票申请；提交后企业信息和订单明细均为不可变快照';
COMMENT ON TABLE invoice_application_items IS '开票申请关联的可开票充值记录；全局唯一约束防止重复开票';
