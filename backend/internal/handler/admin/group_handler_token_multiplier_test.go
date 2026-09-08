//go:build unit

package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupTokenMultiplierRequestOmissionCompatibility(t *testing.T) {
	var create CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"legacy","rate_multiplier":1}`), &create))
	require.Nil(t, create.InputTokenMultiplier)
	require.Nil(t, create.OutputTokenMultiplier)
	require.Nil(t, create.CacheCreationTokenMultiplier)
	require.Nil(t, create.CacheReadTokenMultiplier)
	require.False(t, create.ReturnBillableUsage)

	var update UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"legacy"}`), &update))
	require.Nil(t, update.InputTokenMultiplier)
	require.Nil(t, update.OutputTokenMultiplier)
	require.Nil(t, update.CacheCreationTokenMultiplier)
	require.Nil(t, update.CacheReadTokenMultiplier)
	require.Nil(t, update.ReturnBillableUsage)
}

func TestGroupTokenMultiplierRequestPreservesExplicitZero(t *testing.T) {
	var create CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"name":"zero",
		"rate_multiplier":1,
		"input_token_multiplier":0,
		"output_token_multiplier":0,
		"cache_creation_token_multiplier":0,
		"cache_read_token_multiplier":0,
		"return_billable_usage":true
	}`), &create))
	require.NotNil(t, create.InputTokenMultiplier)
	require.Zero(t, *create.InputTokenMultiplier)
	require.NotNil(t, create.OutputTokenMultiplier)
	require.Zero(t, *create.OutputTokenMultiplier)
	require.NotNil(t, create.CacheCreationTokenMultiplier)
	require.Zero(t, *create.CacheCreationTokenMultiplier)
	require.NotNil(t, create.CacheReadTokenMultiplier)
	require.Zero(t, *create.CacheReadTokenMultiplier)
	require.True(t, create.ReturnBillableUsage)
}
