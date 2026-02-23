package instruments

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
)

// Master manages symbol to instrument ID mappings
type Master struct {
	symbolToInstrumentID map[string]int
	instrumentIDToSymbol map[int]string
	mu                   sync.RWMutex
	logger               *zap.Logger
}

// NewMaster creates a new instrument master
func NewMaster(logger *zap.Logger) *Master {
	return &Master{
		symbolToInstrumentID: make(map[string]int),
		instrumentIDToSymbol: make(map[int]string),
		logger:               logger,
	}
}

// LoadFromFile loads symbol mappings from a JSON file
func (m *Master) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read symbol mapping file: %w", err)
	}

	var mappings map[string]int
	if err := json.Unmarshal(data, &mappings); err != nil {
		return fmt.Errorf("failed to unmarshal symbol mappings: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for symbol, instrumentID := range mappings {
		m.symbolToInstrumentID[symbol] = instrumentID
		m.instrumentIDToSymbol[instrumentID] = symbol
	}

	m.logger.Info("Loaded symbol mappings", zap.Int("count", len(mappings)))

	return nil
}

// GetInstrumentID returns the instrument ID for a symbol
func (m *Master) GetInstrumentID(symbol string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.symbolToInstrumentID[symbol]
	return id, ok
}

// GetSymbol returns the symbol for an instrument ID
func (m *Master) GetSymbol(instrumentID int) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	symbol, ok := m.instrumentIDToSymbol[instrumentID]
	return symbol, ok
}

// AddMapping adds a new symbol to instrument ID mapping
func (m *Master) AddMapping(symbol string, instrumentID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.symbolToInstrumentID[symbol] = instrumentID
	m.instrumentIDToSymbol[instrumentID] = symbol
}

// GetAllMappings returns all symbol mappings
func (m *Master) GetAllMappings() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]int, len(m.symbolToInstrumentID))
	for k, v := range m.symbolToInstrumentID {
		result[k] = v
	}
	return result
}
