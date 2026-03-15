package loxone

import (
	"testing"
)

func TestParseText_On(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseText("SET switch_1 ON")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Type != "command" {
		t.Errorf("expected type 'command', got %q", cmd.Type)
	}
	if cmd.Target != "switch_1" {
		t.Errorf("expected target 'switch_1', got %q", cmd.Target)
	}
	on, ok := cmd.Params["on"].(bool)
	if !ok || !on {
		t.Errorf("expected Params[on]=true, got %v", cmd.Params["on"])
	}
}

func TestParseText_Off(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseText("SET switch_1 OFF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	on, ok := cmd.Params["on"].(bool)
	if !ok || on {
		t.Errorf("expected Params[on]=false, got %v", cmd.Params["on"])
	}
}

func TestParseText_Brightness(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseText("SET dimmer_1 BRI 75")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bri, ok := cmd.Params["brightness"].(float64)
	if !ok || bri != 75 {
		t.Errorf("expected brightness 75, got %v", cmd.Params["brightness"])
	}
	on, ok := cmd.Params["on"].(bool)
	if !ok || !on {
		t.Errorf("expected Params[on]=true for brightness > 0, got %v", cmd.Params["on"])
	}
}

func TestParseText_BrightnessZero(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseText("SET dimmer_1 BRI 0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bri, ok := cmd.Params["brightness"].(float64)
	if !ok || bri != 0 {
		t.Errorf("expected brightness 0, got %v", cmd.Params["brightness"])
	}
	// No implicit on=true for brightness 0
	_, hasOn := cmd.Params["on"]
	if hasOn {
		t.Errorf("expected no 'on' param for brightness=0, got %v", cmd.Params["on"])
	}
}

func TestParseText_Position(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseText("SET roller_1 POS 50")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pos, ok := cmd.Params["position"].(int)
	if !ok || pos != 50 {
		t.Errorf("expected position 50, got %v", cmd.Params["position"])
	}
}

func TestParseText_Get(t *testing.T) {
	p := NewCommandParser()
	cmd, err := p.ParseText("GET switch_1 STATUS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Type != "query" {
		t.Errorf("expected type 'query', got %q", cmd.Type)
	}
	if cmd.Target != "switch_1" {
		t.Errorf("expected target 'switch_1', got %q", cmd.Target)
	}
}

func TestParseText_TooFewParts(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseText("SET switch_1")
	if err == nil {
		t.Error("expected error for too few parts")
	}
}

func TestParseText_UnknownCommandType(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseText("DELETE switch_1 ON")
	if err == nil {
		t.Error("expected error for unknown command type")
	}
}

func TestParseText_UnknownAction(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseText("SET switch_1 TOGGLE")
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestParseText_BrightnessMissingValue(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseText("SET dimmer_1 BRI")
	if err == nil {
		t.Error("expected error when brightness value is missing")
	}
}

func TestParseText_BrightnessInvalidValue(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseText("SET dimmer_1 BRI abc")
	if err == nil {
		t.Error("expected error for non-numeric brightness value")
	}
}

func TestParseText_PositionMissingValue(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseText("SET roller_1 POS")
	if err == nil {
		t.Error("expected error when position value is missing")
	}
}

func TestParseText_PositionInvalidValue(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseText("SET roller_1 POS xyz")
	if err == nil {
		t.Error("expected error for non-integer position value")
	}
}

func TestParseJSON_Valid(t *testing.T) {
	p := NewCommandParser()
	data := []byte(`{"type":"command","target":"switch_1","action":"set","params":{"on":true}}`)
	cmd, err := p.ParseJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Type != "command" {
		t.Errorf("expected type 'command', got %q", cmd.Type)
	}
	if cmd.Target != "switch_1" {
		t.Errorf("expected target 'switch_1', got %q", cmd.Target)
	}
}

func TestParseJSON_Invalid(t *testing.T) {
	p := NewCommandParser()
	_, err := p.ParseJSON([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestToDeviceCommand_OnOff(t *testing.T) {
	p := NewCommandParser()
	cmd, _ := p.ParseText("SET switch_1 ON")
	dc := p.ToDeviceCommand(cmd)
	if dc.On == nil || !*dc.On {
		t.Errorf("expected On=true, got %v", dc.On)
	}
}

func TestToDeviceCommand_Brightness(t *testing.T) {
	p := NewCommandParser()
	cmd, _ := p.ParseText("SET dimmer_1 BRI 80")
	dc := p.ToDeviceCommand(cmd)
	if dc.Brightness == nil || *dc.Brightness != 80.0 {
		t.Errorf("expected Brightness=80, got %v", dc.Brightness)
	}
}

func TestToDeviceCommand_Position(t *testing.T) {
	p := NewCommandParser()
	cmd, _ := p.ParseText("SET roller_1 POS 30")
	dc := p.ToDeviceCommand(cmd)
	if dc.Position == nil || *dc.Position != 30 {
		t.Errorf("expected Position=30, got %v", dc.Position)
	}
}
