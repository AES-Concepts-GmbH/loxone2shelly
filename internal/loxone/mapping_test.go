package loxone

import (
	"testing"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

func newTestMapping(id, loxoneID, shellyID string, enabled bool) *models.Mapping {
	return &models.Mapping{
		ID:           id,
		LoxoneID:     loxoneID,
		ShellyID:     shellyID,
		ShellyType:   "switch",
		MiniserverID: "ms1",
		Enabled:      enabled,
	}
}

func TestMappingManager_AddAndGetByLoxoneID(t *testing.T) {
	m := NewMappingManager()
	mapping := newTestMapping("id1", "lox1", "shelly1", true)
	m.Add(mapping)

	got := m.GetByLoxoneID("lox1")
	if got == nil {
		t.Fatal("expected mapping, got nil")
	}
	if got.ShellyID != "shelly1" {
		t.Errorf("expected ShellyID 'shelly1', got %q", got.ShellyID)
	}
}

func TestMappingManager_AddAndGetByShellyID(t *testing.T) {
	m := NewMappingManager()
	mapping := newTestMapping("id1", "lox1", "shelly1", true)
	m.Add(mapping)

	got := m.GetByShellyID("shelly1")
	if got == nil {
		t.Fatal("expected mapping, got nil")
	}
	if got.LoxoneID != "lox1" {
		t.Errorf("expected LoxoneID 'lox1', got %q", got.LoxoneID)
	}
}

func TestMappingManager_DisabledMappingNotIndexed(t *testing.T) {
	m := NewMappingManager()
	mapping := newTestMapping("id1", "lox1", "shelly1", false)
	m.Add(mapping)

	if got := m.GetByLoxoneID("lox1"); got != nil {
		t.Error("expected nil for disabled mapping by LoxoneID")
	}
	if got := m.GetByShellyID("shelly1"); got != nil {
		t.Error("expected nil for disabled mapping by ShellyID")
	}
}

func TestMappingManager_Remove(t *testing.T) {
	m := NewMappingManager()
	mapping := newTestMapping("id1", "lox1", "shelly1", true)
	m.Add(mapping)
	m.Remove("id1")

	if got := m.GetByLoxoneID("lox1"); got != nil {
		t.Error("expected nil after removal")
	}
	if got := m.GetByShellyID("shelly1"); got != nil {
		t.Error("expected nil after removal")
	}
}

func TestMappingManager_RemoveNonExistent(t *testing.T) {
	m := NewMappingManager()
	// Should not panic
	m.Remove("non-existent-id")
}

func TestMappingManager_Load(t *testing.T) {
	m := NewMappingManager()
	mappings := []models.Mapping{
		{ID: "a", LoxoneID: "lox_a", ShellyID: "s_a", ShellyType: "switch", Enabled: true},
		{ID: "b", LoxoneID: "lox_b", ShellyID: "s_b", ShellyType: "light", Enabled: false},
	}
	m.Load(mappings)

	if got := m.GetByLoxoneID("lox_a"); got == nil {
		t.Error("expected mapping for lox_a")
	}
	if got := m.GetByLoxoneID("lox_b"); got != nil {
		t.Error("expected nil for disabled mapping lox_b")
	}
}

func TestMappingManager_LoadReplacesExisting(t *testing.T) {
	m := NewMappingManager()
	m.Add(newTestMapping("old", "lox_old", "shelly_old", true))

	m.Load([]models.Mapping{
		{ID: "new", LoxoneID: "lox_new", ShellyID: "shelly_new", ShellyType: "switch", Enabled: true},
	})

	if got := m.GetByLoxoneID("lox_old"); got != nil {
		t.Error("expected old mapping to be replaced by Load")
	}
	if got := m.GetByLoxoneID("lox_new"); got == nil {
		t.Error("expected new mapping after Load")
	}
}

func TestMappingManager_GetAll(t *testing.T) {
	m := NewMappingManager()
	m.Add(newTestMapping("id1", "lox1", "shelly1", true))
	m.Add(newTestMapping("id2", "lox2", "shelly2", true))

	all := m.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 mappings, got %d", len(all))
	}
}

func TestMappingManager_ResolveTarget(t *testing.T) {
	m := NewMappingManager()
	m.Add(newTestMapping("id1", "lox1", "shelly1", true))

	shellyID, shellyType, ok := m.ResolveTarget("lox1")
	if !ok {
		t.Fatal("expected ok=true for known target")
	}
	if shellyID != "shelly1" {
		t.Errorf("expected shellyID 'shelly1', got %q", shellyID)
	}
	if shellyType != "switch" {
		t.Errorf("expected shellyType 'switch', got %q", shellyType)
	}
}

func TestMappingManager_ResolveTargetUnknown(t *testing.T) {
	m := NewMappingManager()
	_, _, ok := m.ResolveTarget("unknown")
	if ok {
		t.Error("expected ok=false for unknown target")
	}
}
