package loxone

import (
	"sync"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// MappingManager handles Loxone to Shelly resource mappings
type MappingManager struct {
	mappings  map[string]*models.Mapping // keyed by LoxoneID
	byShellyID map[string]*models.Mapping // keyed by ShellyID
	mu        sync.RWMutex
}

// NewMappingManager creates a new mapping manager
func NewMappingManager() *MappingManager {
	return &MappingManager{
		mappings:  make(map[string]*models.Mapping),
		byShellyID: make(map[string]*models.Mapping),
	}
}

// Load initializes mappings from a list
func (m *MappingManager) Load(mappings []models.Mapping) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.mappings = make(map[string]*models.Mapping)
	m.byShellyID = make(map[string]*models.Mapping)

	for i := range mappings {
		mapping := &mappings[i]
		if mapping.Enabled {
			m.mappings[mapping.LoxoneID] = mapping
			m.byShellyID[mapping.ShellyID] = mapping
		}
	}
}

// GetByLoxoneID returns a mapping by Loxone ID
func (m *MappingManager) GetByLoxoneID(loxoneID string) *models.Mapping {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mappings[loxoneID]
}

// GetByShellyID returns a mapping by Shelly ID
func (m *MappingManager) GetByShellyID(shellyID string) *models.Mapping {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byShellyID[shellyID]
}

// Add adds a new mapping
func (m *MappingManager) Add(mapping *models.Mapping) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mapping.Enabled {
		m.mappings[mapping.LoxoneID] = mapping
		m.byShellyID[mapping.ShellyID] = mapping
	}
}

// Remove removes a mapping by ID
func (m *MappingManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for loxoneID, mapping := range m.mappings {
		if mapping.ID == id {
			delete(m.mappings, loxoneID)
			delete(m.byShellyID, mapping.ShellyID)
			return
		}
	}
}

// GetAll returns all mappings
func (m *MappingManager) GetAll() []models.Mapping {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.Mapping, 0, len(m.mappings))
	for _, mapping := range m.mappings {
		result = append(result, *mapping)
	}
	return result
}

// ResolveTarget resolves a Loxone target ID to Shelly resource info
func (m *MappingManager) ResolveTarget(target string) (shellyID, shellyType string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if mapping, exists := m.mappings[target]; exists && mapping.Enabled {
		return mapping.ShellyID, mapping.ShellyType, true
	}
	return "", "", false
}
