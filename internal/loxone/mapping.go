package loxone

import (
	"sync"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// MappingManager handles Loxone to Shelly resource mappings.
// Only enabled mappings are indexed.
type MappingManager struct {
	byLoxoneID map[string]*models.Mapping   // keyed by LoxoneID
	byShellyID map[string][]*models.Mapping // keyed by ShellyID, all mappings of that device
	mu         sync.RWMutex
}

// NewMappingManager creates a new mapping manager
func NewMappingManager() *MappingManager {
	return &MappingManager{
		byLoxoneID: make(map[string]*models.Mapping),
		byShellyID: make(map[string][]*models.Mapping),
	}
}

// Load replaces all mappings. The manager keeps its own copies.
func (m *MappingManager) Load(mappings []models.Mapping) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.byLoxoneID = make(map[string]*models.Mapping)
	m.byShellyID = make(map[string][]*models.Mapping)

	for _, src := range mappings {
		if !src.Enabled {
			continue
		}
		mapping := src.Clone()
		mp := &mapping
		m.byLoxoneID[mp.LoxoneID] = mp
		m.byShellyID[mp.ShellyID] = append(m.byShellyID[mp.ShellyID], mp)
	}
}

// GetByLoxoneID returns a copy of the mapping for a Loxone ID, or nil.
func (m *MappingManager) GetByLoxoneID(loxoneID string) *models.Mapping {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mp, ok := m.byLoxoneID[loxoneID]; ok {
		cp := mp.Clone()
		return &cp
	}
	return nil
}

// GetByShellyID returns copies of all enabled mappings for a Shelly device.
func (m *MappingManager) GetByShellyID(shellyID string) []models.Mapping {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.byShellyID[shellyID]
	result := make([]models.Mapping, 0, len(list))
	for _, mp := range list {
		result = append(result, mp.Clone())
	}
	return result
}

// GetForChannel returns the mappings that apply to a state update of the given
// device. channel == nil selects device-level mappings (no channel set).
// For a channel update, channel-specific mappings win; channel 0 additionally
// falls back to device-level mappings so single-channel setups keep working.
func (m *MappingManager) GetForChannel(shellyID string, channel *int) []models.Mapping {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var specific, deviceLevel []models.Mapping
	for _, mp := range m.byShellyID[shellyID] {
		switch {
		case mp.Channel == nil:
			deviceLevel = append(deviceLevel, mp.Clone())
		case channel != nil && *mp.Channel == *channel:
			specific = append(specific, mp.Clone())
		}
	}

	if channel == nil {
		return deviceLevel
	}
	if len(specific) > 0 {
		return specific
	}
	if *channel == 0 {
		return deviceLevel
	}
	return nil
}

// Remove removes a mapping by ID
func (m *MappingManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for loxoneID, mapping := range m.byLoxoneID {
		if mapping.ID != id {
			continue
		}
		delete(m.byLoxoneID, loxoneID)
		list := m.byShellyID[mapping.ShellyID]
		filtered := list[:0]
		for _, mp := range list {
			if mp.ID != id {
				filtered = append(filtered, mp)
			}
		}
		if len(filtered) == 0 {
			delete(m.byShellyID, mapping.ShellyID)
		} else {
			m.byShellyID[mapping.ShellyID] = filtered
		}
		return
	}
}

// GetAll returns copies of all enabled mappings
func (m *MappingManager) GetAll() []models.Mapping {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.Mapping, 0, len(m.byLoxoneID))
	for _, mapping := range m.byLoxoneID {
		result = append(result, mapping.Clone())
	}
	return result
}

// ResolveTarget resolves a Loxone target ID to the Shelly device, type and
// optional channel it controls.
func (m *MappingManager) ResolveTarget(target string) (shellyID, shellyType string, channel *int, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mapping, exists := m.byLoxoneID[target]
	if !exists {
		return "", "", nil, false
	}
	if mapping.Channel != nil {
		ch := *mapping.Channel
		channel = &ch
	}
	return mapping.ShellyID, mapping.ShellyType, channel, true
}
