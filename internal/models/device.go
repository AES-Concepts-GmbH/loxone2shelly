package models

// Device represents a Shelly device
type Device struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Type         string       `json:"type"`  // "switch", "light", "dimmer", "roller", "rgb", "rgbw", "sensor", "meter"
	Model        string       `json:"model"` // e.g. "SNSW-001X16EU" (Shelly Plus 1)
	IP           string       `json:"ip"`
	Generation   int          `json:"generation"` // 1 or 2+
	FirmwareVer  string       `json:"firmware_ver"`
	State        DeviceState  `json:"state"`
	Capabilities Capabilities `json:"capabilities,omitempty"`
	Online       bool         `json:"online"`
	Channels     []Channel    `json:"channels,omitempty"` // Multi-channel devices (Pro 2PM, Pro 4PM, etc.)
}

// DeviceState represents the current state of a Shelly device (primary channel)
type DeviceState struct {
	On          bool     `json:"on"`
	Brightness  float64  `json:"brightness,omitempty"`  // 0-100
	ColorTemp   int      `json:"color_temp,omitempty"`  // Kelvin (3000-6500)
	Red         int      `json:"red,omitempty"`         // 0-255
	Green       int      `json:"green,omitempty"`       // 0-255
	Blue        int      `json:"blue,omitempty"`        // 0-255
	White       int      `json:"white,omitempty"`       // 0-255
	Position    int      `json:"position,omitempty"`    // 0-100 (roller)
	Power       float64  `json:"power,omitempty"`       // Watts
	Voltage     float64  `json:"voltage,omitempty"`     // V
	Current     float64  `json:"current,omitempty"`     // A
	Energy      float64  `json:"energy,omitempty"`      // Wh
	Temperature *float64 `json:"temperature,omitempty"` // °C
	Humidity    *float64 `json:"humidity,omitempty"`    // %RH
	Illuminance *float64 `json:"illuminance,omitempty"` // Lux
	Battery     *float64 `json:"battery,omitempty"`     // % (BLU devices)
	Smoke       *bool    `json:"smoke,omitempty"`       // Smoke alarm active
	Flood       *bool    `json:"flood,omitempty"`       // Flood alarm active
	// Energy meter fields (3-phase)
	TotalPower  float64 `json:"total_power,omitempty"`  // Total active power (W) across phases
	PowerA      float64 `json:"power_a,omitempty"`      // Phase A power (W)
	PowerB      float64 `json:"power_b,omitempty"`      // Phase B power (W)
	PowerC      float64 `json:"power_c,omitempty"`      // Phase C power (W)
	VoltageA    float64 `json:"voltage_a,omitempty"`    // Phase A voltage (V)
	VoltageB    float64 `json:"voltage_b,omitempty"`    // Phase B voltage (V)
	VoltageC    float64 `json:"voltage_c,omitempty"`    // Phase C voltage (V)
	CurrentA    float64 `json:"current_a,omitempty"`    // Phase A current (A)
	CurrentB    float64 `json:"current_b,omitempty"`    // Phase B current (A)
	CurrentC    float64 `json:"current_c,omitempty"`    // Phase C current (A)
	TotalEnergy float64 `json:"total_energy,omitempty"` // Total energy (Wh)
}

// Channel represents a single channel in a multi-channel device
type Channel struct {
	Index      int     `json:"index"`
	Type       string  `json:"type"` // "switch", "light", "cover"
	On         bool    `json:"on"`
	Brightness float64 `json:"brightness,omitempty"`
	Position   int     `json:"position,omitempty"`
	Power      float64 `json:"power,omitempty"`
	Voltage    float64 `json:"voltage,omitempty"`
	Current    float64 `json:"current,omitempty"`
	Energy     float64 `json:"energy,omitempty"`
}

// Capabilities describes what a Shelly device can do
type Capabilities struct {
	SupportsColor       bool `json:"supports_color"`
	SupportsColorTemp   bool `json:"supports_color_temp"`
	SupportsDimming     bool `json:"supports_dimming"`
	SupportsRoller      bool `json:"supports_roller"`
	SupportsPower       bool `json:"supports_power"`
	SupportsTemperature bool `json:"supports_temperature"`
	SupportsHumidity    bool `json:"supports_humidity"`
	SupportsIlluminance bool `json:"supports_illuminance"`
	SupportsBattery     bool `json:"supports_battery"`
	SupportsSmoke       bool `json:"supports_smoke"`
	SupportsFlood       bool `json:"supports_flood"`
	SupportsEnergyMeter bool `json:"supports_energy_meter"`
	MultiChannel        bool `json:"multi_channel"`
	ChannelCount        int  `json:"channel_count,omitempty"`
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
	Stop       *bool    `json:"stop,omitempty"`    // Stop a moving cover
	Channel    *int     `json:"channel,omitempty"` // Target channel for multi-channel devices
}

// Prop is a single Loxone feedback property (name/value pair).
type Prop struct {
	Name  string
	Value interface{}
}
