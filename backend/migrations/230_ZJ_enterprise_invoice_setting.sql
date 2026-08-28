-- 企业开票功能总开关，默认关闭。
-- 使用 ON CONFLICT 保留已有实例的管理员配置。
INSERT INTO settings (key, value, updated_at)
VALUES ('enterprise_invoice_enabled', 'false', NOW())
ON CONFLICT (key) DO NOTHING;
