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
	return &cmd, nil
}

// ParseText parses a simple text command for Shelly devices.
// Format: SET <target> <property> [value]
// Examples:
//   - SET switch_1 ON
//   - SET switch_1 OFF
//   - SET dimmer_1 BRI 80
//   - SET roller_1 POS 50
//   - GET switch_1 STATUS
func (p *CommandParser) ParseText(text string) (*models.LoxoneCommand, error) {
	parts := strings.Fields(text)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid command format: %s (expected: SET/GET <target> <action> [value])", text)
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
			if err != nil {
				return nil, fmt.Errorf("invalid brightness value: %s", parts[3])
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
			if err != nil {
				return nil, fmt.Errorf("invalid position value: %s", parts[3])
			}
			cmd.Params["position"] = pos
		default:
			return nil, fmt.Errorf("unknown action: %s (supported: ON, OFF, BRI, POS)", action)
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
// suitable for sending to a Shelly device.
func (p *CommandParser) ToDeviceCommand(cmd *models.LoxoneCommand) models.DeviceCommand {
	dc := models.DeviceCommand{}

	if on, ok := cmd.Params["on"].(bool); ok {
		dc.On = &on
	}

	if bri, ok := cmd.Params["brightness"].(float64); ok {
		dc.Brightness = &bri
	}

	if pos, ok := cmd.Params["position"].(int); ok {
		dc.Position = &pos
	}
	// Handle position as float64 (from JSON unmarshaling)
	if pos, ok := cmd.Params["position"].(float64); ok {
		posInt := int(pos)
		dc.Position = &posInt
	}

	return dc
}
