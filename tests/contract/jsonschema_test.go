package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadSchema(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(name)
	require.NoError(t, err, "reading schema file %s", name)

	var schema map[string]interface{}
	err = json.Unmarshal(data, &schema)
	require.NoError(t, err, "parsing schema file %s", name)

	return schema
}

func TestContractSchemasAreValidJSON(t *testing.T) {
	contractsDir := filepath.Join("..", "..", "specs", "001-oncall-mcp-server", "contracts")
	entries, err := os.ReadDir(contractsDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".schema.json") {
			continue
		}

		path := filepath.Join(contractsDir, entry.Name())
		schema := loadSchema(t, path)

		assert.NotNil(t, schema["$schema"], "file %s should have $schema", entry.Name())
	}
}

func TestContractSchemasFileCount(t *testing.T) {
	contractsDir := filepath.Join("..", "..", "specs", "001-oncall-mcp-server", "contracts")
	entries, err := os.ReadDir(contractsDir)
	require.NoError(t, err)

	schemaCount := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".schema.json") {
			schemaCount++
		}
	}

	assert.Equal(t, 24, schemaCount,
		"expected 24 contract schema files (22 tool I/O + _defs + error_envelope)")
}

func TestToolSchemasExist(t *testing.T) {
	tools := []string{
		"list_oncall_schedules",
		"get_oncall_shift",
		"get_current_oncall_users",
		"list_oncall_teams",
		"list_oncall_users",
		"list_alert_groups",
		"get_alert_group",
		"acknowledge_alert_group",
		"resolve_alert_group",
		"silence_alert_group",
		"unresolve_alert_group",
	}

	contractsDir := filepath.Join("..", "..", "specs", "001-oncall-mcp-server", "contracts")

	for _, tool := range tools {
		inputPath := filepath.Join(contractsDir, fmt.Sprintf("%s.input.schema.json", tool))
		outputPath := filepath.Join(contractsDir, fmt.Sprintf("%s.output.schema.json", tool))

		_, err := os.Stat(inputPath)
		assert.NoError(t, err, "missing input schema for tool %s at %s", tool, inputPath)

		_, err = os.Stat(outputPath)
		assert.NoError(t, err, "missing output schema for tool %s at %s", tool, outputPath)
	}
}

func TestErrorEnvelopeSchemaExists(t *testing.T) {
	contractsDir := filepath.Join("..", "..", "specs", "001-oncall-mcp-server", "contracts")
	path := filepath.Join(contractsDir, "error_envelope.schema.json")
	loadSchema(t, path)
}

func TestDefsSchemaExists(t *testing.T) {
	contractsDir := filepath.Join("..", "..", "specs", "001-oncall-mcp-server", "contracts")
	path := filepath.Join(contractsDir, "_defs.schema.json")
	loadSchema(t, path)
}

// TestErrorEnvelope002AcceptsBothPluginCodes is the contract test for
// the dual-plugin error-code rename (spec 002-multi-plugin-support,
// Decision 13). The 002 error_envelope.schema.json MUST list both
// `ONCALL_PLUGIN_MISSING` and the deprecated `IRM_PLUGIN_MISSING` alias
// in the `code` enum so that:
//   - new clients that pin the new code continue to validate, AND
//   - old clients that pin the legacy code (e.g. operator scripts
//     that grep for IRM_PLUGIN_MISSING) do not break during the
//     one-minor-release deprecation window mandated by constitution
//     Principle III.
//
// The server never emits `IRM_PLUGIN_MISSING`; this test only
// verifies the schema's promise to accept it.
func TestErrorEnvelope002AcceptsBothPluginCodes(t *testing.T) {
	contractsDir := filepath.Join("..", "..", "specs", "002-multi-plugin-support", "contracts")
	path := filepath.Join(contractsDir, "error_envelope.schema.json")
	schema := loadSchema(t, path)

	codeProp, ok := schema["properties"].(map[string]interface{})["code"].(map[string]interface{})
	require.True(t, ok, "error_envelope.schema.json must have a code property")
	enum, ok := codeProp["enum"].([]interface{})
	require.True(t, ok, "code must be an enum")

	var found []string
	for _, v := range enum {
		if s, ok := v.(string); ok {
			found = append(found, s)
		}
	}
	assert.Contains(t, found, "ONCALL_PLUGIN_MISSING", "002 schema must list the new code as an allowed value")
	assert.Contains(t, found, "IRM_PLUGIN_MISSING", "002 schema must keep the legacy code as a one-minor-release deprecation alias")
}
