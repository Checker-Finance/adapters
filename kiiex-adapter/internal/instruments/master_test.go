package instruments

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMaster_LoadFromFile(t *testing.T) {
	logger := zap.NewNop()
	master := NewMaster(logger)

	// Create a temporary file
	content := `{
		"BTCUSD": 1,
		"ETHUSD": 2,
		"LTCUSD": 3
	}`
	tmpFile, err := os.CreateTemp("", "symbol_mapping_*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	// Load the file
	err = master.LoadFromFile(tmpFile.Name())
	require.NoError(t, err)

	// Verify mappings
	id, ok := master.GetInstrumentID("BTCUSD")
	assert.True(t, ok)
	assert.Equal(t, 1, id)

	id, ok = master.GetInstrumentID("ETHUSD")
	assert.True(t, ok)
	assert.Equal(t, 2, id)

	id, ok = master.GetInstrumentID("LTCUSD")
	assert.True(t, ok)
	assert.Equal(t, 3, id)

	// Verify reverse mappings
	symbol, ok := master.GetSymbol(1)
	assert.True(t, ok)
	assert.Equal(t, "BTCUSD", symbol)

	symbol, ok = master.GetSymbol(2)
	assert.True(t, ok)
	assert.Equal(t, "ETHUSD", symbol)
}

func TestMaster_GetInstrumentID_NotFound(t *testing.T) {
	logger := zap.NewNop()
	master := NewMaster(logger)

	id, ok := master.GetInstrumentID("UNKNOWN")
	assert.False(t, ok)
	assert.Equal(t, 0, id)
}

func TestMaster_GetSymbol_NotFound(t *testing.T) {
	logger := zap.NewNop()
	master := NewMaster(logger)

	symbol, ok := master.GetSymbol(999)
	assert.False(t, ok)
	assert.Equal(t, "", symbol)
}

func TestMaster_AddMapping(t *testing.T) {
	logger := zap.NewNop()
	master := NewMaster(logger)

	master.AddMapping("BTCUSD", 1)

	id, ok := master.GetInstrumentID("BTCUSD")
	assert.True(t, ok)
	assert.Equal(t, 1, id)

	symbol, ok := master.GetSymbol(1)
	assert.True(t, ok)
	assert.Equal(t, "BTCUSD", symbol)
}

func TestMaster_GetAllMappings(t *testing.T) {
	logger := zap.NewNop()
	master := NewMaster(logger)

	master.AddMapping("BTCUSD", 1)
	master.AddMapping("ETHUSD", 2)

	mappings := master.GetAllMappings()
	assert.Len(t, mappings, 2)
	assert.Equal(t, 1, mappings["BTCUSD"])
	assert.Equal(t, 2, mappings["ETHUSD"])
}

func TestMaster_LoadFromFile_FileNotFound(t *testing.T) {
	logger := zap.NewNop()
	master := NewMaster(logger)

	err := master.LoadFromFile("/nonexistent/file.json")
	assert.Error(t, err)
}

func TestMaster_LoadFromFile_InvalidJSON(t *testing.T) {
	logger := zap.NewNop()
	master := NewMaster(logger)

	// Create a temporary file with invalid JSON
	tmpFile, err := os.CreateTemp("", "invalid_*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("invalid json")
	require.NoError(t, err)
	tmpFile.Close()

	err = master.LoadFromFile(tmpFile.Name())
	assert.Error(t, err)
}
