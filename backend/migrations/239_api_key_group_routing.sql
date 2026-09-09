-- Ordered per-key group routing chains. api_keys.group_id remains a mirror of
-- the first entry so older binaries and API clients keep their current view.
CREATE TABLE IF NOT EXISTS api_key_groups (
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (api_key_id, group_id),
    UNIQUE (api_key_id, sort_order)
);

CREATE INDEX IF NOT EXISTS idx_api_key_groups_group_id
    ON api_key_groups (group_id);
CREATE INDEX IF NOT EXISTS idx_api_key_groups_api_key_order
    ON api_key_groups (api_key_id, sort_order, group_id);

INSERT INTO api_key_groups (api_key_id, group_id, sort_order)
SELECT id, group_id, 0
FROM api_keys
WHERE group_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Older binaries only write api_keys.group_id. Replace the chain for those
-- writes, while allowing a new binary to promote a relation-table successor
-- before updating the compatibility mirror.
CREATE OR REPLACE FUNCTION sync_legacy_api_key_group_binding()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    first_group_id BIGINT;
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.group_id IS NOT DISTINCT FROM NEW.group_id THEN
        RETURN NEW;
    END IF;

    SELECT group_id INTO first_group_id
    FROM api_key_groups
    WHERE api_key_id = NEW.id
    ORDER BY sort_order, group_id
    LIMIT 1;

    IF NEW.group_id IS NOT NULL AND first_group_id IS NOT DISTINCT FROM NEW.group_id THEN
        RETURN NEW;
    END IF;
    IF NEW.group_id IS NULL AND first_group_id IS NULL THEN
        RETURN NEW;
    END IF;

    DELETE FROM api_key_groups WHERE api_key_id = NEW.id;
    IF NEW.group_id IS NOT NULL THEN
        INSERT INTO api_key_groups (api_key_id, group_id, sort_order)
        VALUES (NEW.id, NEW.group_id, 0);
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_api_keys_sync_group_bindings ON api_keys;
CREATE TRIGGER trg_api_keys_sync_group_bindings
AFTER INSERT OR UPDATE OF group_id ON api_keys
FOR EACH ROW EXECUTE FUNCTION sync_legacy_api_key_group_binding();

-- Binding edits must invalidate cached auth snapshots on every instance.
CREATE OR REPLACE FUNCTION enqueue_api_key_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_api_key_id BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_api_key_id := OLD.api_key_id;
    ELSE
        target_api_key_id := NEW.api_key_id;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.id = target_api_key_id
      AND k.deleted_at IS NULL
      AND k.key <> '';

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_api_key_groups_auth_cache_invalidation ON api_key_groups;
CREATE TRIGGER trg_api_key_groups_auth_cache_invalidation
AFTER INSERT OR UPDATE OR DELETE ON api_key_groups
FOR EACH ROW EXECUTE FUNCTION enqueue_api_key_group_auth_cache_invalidation();

-- Existing group and permission triggers only follow api_keys.group_id. These
-- supplemental triggers cover every fallback binding without changing applied
-- migration files.
CREATE OR REPLACE FUNCTION enqueue_routing_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_group_id BIGINT;
BEGIN
    target_group_id := OLD.id;
    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT DISTINCT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_key_groups AS akg
    JOIN api_keys AS k ON k.id = akg.api_key_id
    WHERE akg.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_groups_routing_auth_cache_invalidation ON groups;
CREATE TRIGGER trg_groups_routing_auth_cache_invalidation
AFTER UPDATE OR DELETE ON groups
FOR EACH ROW EXECUTE FUNCTION enqueue_routing_group_auth_cache_invalidation();

CREATE OR REPLACE FUNCTION enqueue_routing_permission_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_user_id BIGINT;
    target_group_id BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_user_id := OLD.user_id;
        target_group_id := OLD.group_id;
    ELSE
        target_user_id := NEW.user_id;
        target_group_id := NEW.group_id;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT DISTINCT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_key_groups AS akg
    JOIN api_keys AS k ON k.id = akg.api_key_id
    WHERE k.user_id = target_user_id
      AND akg.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key <> '';

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_user_allowed_groups_routing_auth_cache_invalidation ON user_allowed_groups;
CREATE TRIGGER trg_user_allowed_groups_routing_auth_cache_invalidation
AFTER INSERT OR UPDATE OR DELETE ON user_allowed_groups
FOR EACH ROW EXECUTE FUNCTION enqueue_routing_permission_auth_cache_invalidation();

COMMENT ON TABLE api_key_groups IS
    'Ordered groups available to an API key; lower sort_order is attempted first';
