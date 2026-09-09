package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyGroupRoutingMigrationFiltersGroupInvalidations(t *testing.T) {
	content, err := FS.ReadFile("239_api_key_group_routing.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "(to_jsonb(OLD) - ARRAY[ 'name', 'description', 'sort_order', 'created_at', 'updated_at', 'duplicate_operation_id' ]) IS NOT DISTINCT FROM (to_jsonb(NEW) - ARRAY[ 'name', 'description', 'sort_order', 'created_at', 'updated_at', 'duplicate_operation_id' ])")
	require.GreaterOrEqual(t, strings.Count(sql, "k.group_id IS DISTINCT FROM target_group_id"), 2)
	require.Contains(t, sql, "(OLD.user_id, OLD.group_id), (NEW.user_id, NEW.group_id)")
	require.Contains(t, sql, "g.is_exclusive = TRUE")
}
