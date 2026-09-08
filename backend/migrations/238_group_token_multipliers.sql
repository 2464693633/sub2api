-- 分组可分别调整四类 Token 的计费数量，并可选择把计费 Token 返回给下游。
-- usage_logs 保留原 Token 列作为上游真实值；新增列为 NULL 的历史记录按真实值、1 倍率解释。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS input_token_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN IF NOT EXISTS output_token_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN IF NOT EXISTS cache_creation_token_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN IF NOT EXISTS cache_read_token_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN IF NOT EXISTS return_billable_usage BOOLEAN NOT NULL DEFAULT FALSE;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_input_token_multiplier_range' AND conrelid = 'groups'::regclass) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_input_token_multiplier_range
            CHECK (input_token_multiplier BETWEEN 0 AND 100);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_output_token_multiplier_range' AND conrelid = 'groups'::regclass) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_output_token_multiplier_range
            CHECK (output_token_multiplier BETWEEN 0 AND 100);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_cache_creation_token_multiplier_range' AND conrelid = 'groups'::regclass) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_cache_creation_token_multiplier_range
            CHECK (cache_creation_token_multiplier BETWEEN 0 AND 100);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_cache_read_token_multiplier_range' AND conrelid = 'groups'::regclass) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_cache_read_token_multiplier_range
            CHECK (cache_read_token_multiplier BETWEEN 0 AND 100);
    END IF;
END $$;

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS billable_input_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS billable_output_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS billable_cache_creation_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS billable_cache_read_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS input_token_multiplier DECIMAL(10,4),
    ADD COLUMN IF NOT EXISTS output_token_multiplier DECIMAL(10,4),
    ADD COLUMN IF NOT EXISTS cache_creation_token_multiplier DECIMAL(10,4),
    ADD COLUMN IF NOT EXISTS cache_read_token_multiplier DECIMAL(10,4);

-- NOT VALID avoids a full scan of a potentially large usage_logs table while
-- still enforcing the configured range for every newly inserted snapshot.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_input_token_multiplier_range' AND conrelid = 'usage_logs'::regclass) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_input_token_multiplier_range
            CHECK (input_token_multiplier IS NULL OR input_token_multiplier BETWEEN 0 AND 100) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_output_token_multiplier_range' AND conrelid = 'usage_logs'::regclass) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_output_token_multiplier_range
            CHECK (output_token_multiplier IS NULL OR output_token_multiplier BETWEEN 0 AND 100) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_cache_creation_token_multiplier_range' AND conrelid = 'usage_logs'::regclass) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_cache_creation_token_multiplier_range
            CHECK (cache_creation_token_multiplier IS NULL OR cache_creation_token_multiplier BETWEEN 0 AND 100) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'usage_logs_cache_read_token_multiplier_range' AND conrelid = 'usage_logs'::regclass) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_cache_read_token_multiplier_range
            CHECK (cache_read_token_multiplier IS NULL OR cache_read_token_multiplier BETWEEN 0 AND 100) NOT VALID;
    END IF;
END $$;

COMMENT ON COLUMN groups.input_token_multiplier IS '输入 Token 计费倍率，范围 0 到 100';
COMMENT ON COLUMN groups.output_token_multiplier IS '输出 Token 计费倍率，范围 0 到 100';
COMMENT ON COLUMN groups.cache_creation_token_multiplier IS '缓存创建 Token 计费倍率，同时作用于 5m 和 1h';
COMMENT ON COLUMN groups.cache_read_token_multiplier IS '缓存读取 Token 计费倍率，范围 0 到 100';
COMMENT ON COLUMN groups.return_billable_usage IS '是否向下游 API 响应返回计费 Token';
COMMENT ON COLUMN usage_logs.billable_input_tokens IS '应用分组倍率并四舍五入后的输入 Token；NULL 表示历史行回退真实值';
COMMENT ON COLUMN usage_logs.billable_output_tokens IS '应用分组倍率并四舍五入后的输出 Token；NULL 表示历史行回退真实值';
COMMENT ON COLUMN usage_logs.billable_cache_creation_tokens IS '应用分组倍率并四舍五入后的缓存创建 Token；NULL 表示历史行回退真实值';
COMMENT ON COLUMN usage_logs.billable_cache_read_tokens IS '应用分组倍率并四舍五入后的缓存读取 Token；NULL 表示历史行回退真实值';
COMMENT ON COLUMN usage_logs.input_token_multiplier IS '请求结算时的输入 Token 倍率快照';
COMMENT ON COLUMN usage_logs.output_token_multiplier IS '请求结算时的输出 Token 倍率快照';
COMMENT ON COLUMN usage_logs.cache_creation_token_multiplier IS '请求结算时的缓存创建 Token 倍率快照';
COMMENT ON COLUMN usage_logs.cache_read_token_multiplier IS '请求结算时的缓存读取 Token 倍率快照';
