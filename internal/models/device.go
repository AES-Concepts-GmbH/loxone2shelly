package models

// Device represents a Shelly device
type Device struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Type         string       `json:"type"`          // "switch", "light", "dimmer", "roller", "rgb", "rgbw"
	Model        string       `json:"model"`         // e.g. "SNSW-001X16EU" (Shelly Plus 1)
	IP           string       `json:"ip"`
	Generation   int          `json:"generation"`    // 1 or 2+
	FirmwareVer  string       `json:"firmware_ver"`
	State        DeviceState  `json:"state"`
	Capabilities Capabilities `json:"capabilities,omitempty"`
	Online       bool         `json:"online"`
}

// DeviceState represents the current state of a Shelly device
type DeviceState struct {
	On         bool    `json:"on"`
	Brightness float64 `json:"brightness,omitempty"` // 0-100
	ColorTemp  int     `json:"color_temp,omitempty"` // Kelvin (3000-6500)
	Red        int     `json:"red,omitempty"`        // 0-255
	Green      int     `json:"green,omitempty"`      // 0-255
	Blue       int     `json:"blue,omitempty"`       // 0-255
	White      int     `json:"white,omitempty"`      // 0-255
	Position   int     `json:"position,omitempty"`   // 0-100 (roller)
	Power      float64 `json:"power,omitempty"`      // Watts
	Energy     float64 `json:"energy,omitempty"`     // Wh
}

// Capabilities describes what a Shelly device can do
type Capabilities struct {
	SupportsColor     bool `json:"supports_color"`
	SupportsColorTemp bool `json:"supports_color_temp"`
	SupportsDimming   bool `json:"supports_dimming"`
	SupportsRoller    bool `json:"supports_roller"`
	SupportsPower     bool `json:"supports_power"`
}

// DeviceCommand represents a command to control a device
type DeviceCommand struct {
	On         *bool    `json:"on,omitempty"`
	Brightness *float64 `json:"brightness,omitempty"`
	ColorTemp  *int     `json:"color_temp,omitempty"`
	Red        *int     `json:"red,omitempty"`
	Green      *int     `json:"green,omitempty"`
	Blue       *int     `json:"blue,omitempty"`
	White      *int     `json:"white,omitempty"`
	Position   *int     `json:"position,omitempty"`
}
