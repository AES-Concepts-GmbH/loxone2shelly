package loxone

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// CommandParser parses Loxone commands for Shelly devices
type CommandParser struct{}

// NewCommandParser creates a new command parser
func NewCommandParser() *CommandParser {
	return &CommandParser{}
}

// ParseJSON parses a JSON command from Loxone
func (p *CommandParser) ParseJSON(data []byte) (*models.LoxoneCommand, error) {
	var cmd models.LoxoneCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return nil, err
	}
	if cmd.Target == "" {
		return nil, fmt.Errorf("target is required")
	}
	if cmd.Type == "" {
		cmd.Type = "command"
	}
	if cmd.Params == nil {
		cmd.Params = make(map[string]interface{})
	}
	return &cmd, nil
}

// ParseText parses a simple text command for Shelly devices.
// Format: SET <target> <property> [value]
// Examples:
//   - SET switch_1 ON
//   - SET switch_1 OFF
//   - SET dimmer_1 BRI 80
//   - SET roller_1 POS 50
//   - SET roller_1 OPEN / CLOSE / STOP
//   - GET switch_1 STATUS
func (p *CommandParser) ParseText(text string) (*models.LoxoneCommand, error) {
	parts := strings.Fields(text)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid command format: %q (expected: SET/GET <target> <action> [value])", text)
	}

	cmdType := strings.ToUpper(parts[0])
	target := parts[1]
	action := strings.ToUpper(parts[2])

	cmd := &models.LoxoneCommand{
		Target: target,
		Params: make(map[string]interface{}),
	}

	switch cmdType {
	case "SET":
		cmd.Type = "command"
		cmd.Action = "set"

		switch action {
		case "ON":
			cmd.Params["on"] = true
		case "OFF":
			cmd.Params["on"] = false
		case "BRI":
			if len(parts) < 4 {
				return nil, fmt.Errorf("brightness value required")
			}
			bri, err := strconv.ParseFloat(parts[3], 64)
			if err != nil || bri < 0 || bri > 100 {
				return nil, fmt.Errorf("invalid brightness value: %s (0-100)", parts[3])
			}
			cmd.Params["brightness"] = bri
			// Implicitly turn on when setting brightness > 0
			if bri > 0 {
				cmd.Params["on"] = true
			}
		case "POS":
			if len(parts) < 4 {
				return nil, fmt.Errorf("position value required")
			}
			pos, err := strconv.Atoi(parts[3])
			if err != nil || pos < 0 || pos > 100 {
				return nil, fmt.Errorf("invalid position value: %s (0-100)", parts[3])
			}
			cmd.Params["position"] = pos
		// Covers: ON/OFF open and close as well; these are the explicit forms.
		case "OPEN":
			cmd.Params["on"] = true
		case "CLOSE":
			cmd.Params["on"] = false
		case "STOP":
			cmd.Params["stop"] = true
		default:
			return nil, fmt.Errorf("unknown action: %s (supported: ON, OFF, BRI, POS, OPEN, CLOSE, STOP)", action)
		}

	case "GET":
		cmd.Type = "query"
		cmd.Action = action

	default:
		return nil, fmt.Errorf("unknown command type: %s (supported: SET, GET)", cmdType)
	}

	return cmd, nil
}

// ToDeviceCommand converts Loxone command params to a models.DeviceCommand
// suitable for sending to a Shelly device. channel (from the mapping) is
// applied when the command itself does not name one.
func (p *CommandParser) ToDeviceCommand(cmd *models.LoxoneCommand, channel *int) models.DeviceCommand {
	dc := models.DeviceCommand{}

	if on, ok := cmd.Params["on"].(bool); ok {
		dc.On = &on
	}

	if bri, ok := toFloat(cmd.Params["brightness"]); ok {
		dc.Brightness = &bri
	}

	if pos, ok := toInt(cmd.Params["position"]); ok {
		dc.Position = &pos
	}

	if stop, ok := cmd.Params["stop"].(bool); ok && stop {
		dc.Stop = &stop
	}

	// Color channels (0-255) and color temperature (Kelvin)
	if v, ok := toInt(cmd.Params["red"]); ok {
		dc.Red = &v
	}
	if v, ok := toInt(cmd.Params["green"]); ok {
		dc.Green = &v
	}
	if v, ok := toInt(cmd.Params["blue"]); ok {
		dc.Blue = &v
	}
	if v, ok := toInt(cmd.Params["white"]); ok {
		dc.White = &v
	}
	if v, ok := toInt(cmd.Params["color_temp"]); ok {
		dc.ColorTemp = &v
	}

	if ch, ok := toInt(cmd.Params["channel"]); ok {
		dc.Channel = &ch
	} else if channel != nil {
		ch := *channel
		dc.Channel = &ch
	}

	return dc
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func toInt(v interface{}) (int, bool) {
	f, ok := toFloat(v)
	if !ok {
		return 0, false
	}
	return int(f), true
}
