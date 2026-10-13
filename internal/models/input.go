package models

// Input represents a Shelly input (button/switch)
type Input struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	DeviceID string     `json:"device_id"`
	Channel  int        `json:"channel"`
	State    InputState `json:"state"`
}

// InputState represents the current state of an input
type InputState struct {
	State       *bool   `json:"state,omitempty"`       // true=active, false=inactive
	Event       *string `json:"event,omitempty"`       // "btn_down", "btn_up", "single_push", "double_push", "long_push"
	EventCount  *int    `json:"event_count,omitempty"` // event counter
	LastUpdated string  `json:"last_updated,omitempty"`
}
