package shelly

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// componentIndexes returns the sorted indexes of all "<prefix>:<n>" keys in status.
func componentIndexes(status map[string]interface{}, prefix string) []int {
	var idx []int
	for key := range status {
		if !strings.HasPrefix(key, prefix+":") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(key, prefix+":"))
		if err != nil {
			continue
		}
		idx = append(idx, n)
	}
	sort.Ints(idx)
	return idx
}

func component(status map[string]interface{}, prefix string, index int) (map[string]interface{}, bool) {
	m, ok := status[fmt.Sprintf("%s:%d", prefix, index)].(map[string]interface{})
	return m, ok
}

func num(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key].(float64)
	return v, ok
}

// parseGen2Status dynamically parses all Gen2+ status components
func parseGen2Status(status map[string]interface{}, device *models.Device) {
	// Switches
	switchIdx := componentIndexes(status, "switch")
	for _, i := range switchIdx {
		sw, _ := component(status, "switch", i)
		ch := models.Channel{Index: i, Type: "switch"}
		if output, ok := sw["output"].(bool); ok {
			ch.On = output
		}
		hasPM := false
		if power, ok := num(sw, "apower"); ok {
			ch.Power = power
			hasPM = true
		}
		if voltage, ok := num(sw, "voltage"); ok {
			ch.Voltage = voltage
			hasPM = true
		}
		if current, ok := num(sw, "current"); ok {
			ch.Current = current
		}
		if energy, ok := sw["aenergy"].(map[string]interface{}); ok {
			if total, ok := num(energy, "total"); ok {
				ch.Energy = total
			}
		}
		if i == switchIdx[0] {
			device.State.On = ch.On
			device.State.Power = ch.Power
			device.State.Voltage = ch.Voltage
			device.State.Current = ch.Current
			device.State.Energy = ch.Energy
			if hasPM {
				device.Capabilities.SupportsPower = true
			}
		}
		device.Channels = append(device.Channels, ch)
	}
	if len(switchIdx) > 0 && device.Type == "" {
		device.Type = "switch"
	}
	if len(switchIdx) > 1 {
		device.Capabilities.MultiChannel = true
		device.Capabilities.ChannelCount = len(switchIdx)
	}

	// Lights
	lightIdx := componentIndexes(status, "light")
	for _, i := range lightIdx {
		lt, _ := component(status, "light", i)
		ch := models.Channel{Index: i, Type: "light"}
		if output, ok := lt["output"].(bool); ok {
			ch.On = output
		}
		if bri, ok := num(lt, "brightness"); ok {
			ch.Brightness = bri
		}
		hasPM := false
		if power, ok := num(lt, "apower"); ok {
			ch.Power = power
			hasPM = true
		}
		if i == lightIdx[0] {
			device.State.On = ch.On
			device.State.Brightness = ch.Brightness
			device.State.Power = ch.Power
			device.Capabilities.SupportsDimming = true
			if hasPM {
				device.Capabilities.SupportsPower = true
			}
		}
		device.Channels = append(device.Channels, ch)
	}
	if len(lightIdx) > 0 {
		device.Type = "light"
	}
	if len(lightIdx) > 1 {
		device.Capabilities.MultiChannel = true
		device.Capabilities.ChannelCount = len(lightIdx)
	}

	// Covers
	coverIdx := componentIndexes(status, "cover")
	for _, i := range coverIdx {
		cover, _ := component(status, "cover", i)
		ch := models.Channel{Index: i, Type: "cover"}
		if pos, ok := num(cover, "current_pos"); ok {
			ch.Position = int(pos)
		}
		hasPM := false
		if power, ok := num(cover, "apower"); ok {
			ch.Power = power
			hasPM = true
		}
		if state, ok := cover["state"].(string); ok {
			ch.On = state == "opening" || state == "closing"
		}
		if i == coverIdx[0] {
			device.State.Position = ch.Position
			device.State.Power = ch.Power
			device.Capabilities.SupportsRoller = true
			if hasPM {
				device.Capabilities.SupportsPower = true
			}
		}
		device.Channels = append(device.Channels, ch)
	}
	if len(coverIdx) > 0 {
		device.Type = "roller"
	}
	if len(coverIdx) > 1 {
		device.Capabilities.MultiChannel = true
		device.Capabilities.ChannelCount = len(coverIdx)
	}

	// RGB
	if rgb, ok := component(status, "rgb", 0); ok {
		device.Type = "rgb"
		device.Capabilities.SupportsColor = true
		device.Capabilities.SupportsDimming = true
		parseColor(rgb, device)
	}

	// RGBW
	if rgbw, ok := component(status, "rgbw", 0); ok {
		device.Type = "rgbw"
		device.Capabilities.SupportsColor = true
		device.Capabilities.SupportsDimming = true
		parseColor(rgbw, device)
		if w, ok := num(rgbw, "white"); ok {
			device.State.White = int(w)
		}
	}

	// CCT (tunable white)
	if cct, ok := component(status, "cct", 0); ok {
		device.Type = "light"
		device.Capabilities.SupportsDimming = true
		device.Capabilities.SupportsColorTemp = true
		if output, ok := cct["output"].(bool); ok {
			device.State.On = output
		}
		if bri, ok := num(cct, "brightness"); ok {
			device.State.Brightness = bri
		}
		if ct, ok := num(cct, "ct"); ok {
			device.State.ColorTemp = int(ct)
		}
	}

	// --- Sensor components (built-in and add-on with IDs 100+) ---
	for _, i := range componentIndexes(status, "temperature") {
		temp, _ := component(status, "temperature", i)
		if tC, ok := num(temp, "tC"); ok {
			if device.State.Temperature == nil {
				v := tC
				device.State.Temperature = &v
			}
			device.Capabilities.SupportsTemperature = true
		}
	}
	for _, i := range componentIndexes(status, "humidity") {
		hum, _ := component(status, "humidity", i)
		if rh, ok := num(hum, "rh"); ok {
			if device.State.Humidity == nil {
				v := rh
				device.State.Humidity = &v
			}
			device.Capabilities.SupportsHumidity = true
		}
	}
	if ill, ok := component(status, "illuminance", 0); ok {
		if lux, ok := num(ill, "lux"); ok {
			v := lux
			device.State.Illuminance = &v
			device.Capabilities.SupportsIlluminance = true
		}
	}
	if smoke, ok := component(status, "smoke", 0); ok {
		if alarm, ok := smoke["alarm"].(bool); ok {
			v := alarm
			device.State.Smoke = &v
			device.Capabilities.SupportsSmoke = true
		}
	}
	if flood, ok := component(status, "flood", 0); ok {
		if alarm, ok := flood["alarm"].(bool); ok {
			v := alarm
			device.State.Flood = &v
			device.Capabilities.SupportsFlood = true
		}
	}
	if dp, ok := component(status, "devicepower", 0); ok {
		if bat, ok := dp["battery"].(map[string]interface{}); ok {
			if pct, ok := num(bat, "percent"); ok {
				v := pct
				device.State.Battery = &v
				device.Capabilities.SupportsBattery = true
			}
		}
	}

	// --- Energy meter components ---

	// 3-phase energy meter (em:0) - Pro 3EM, 3EM Gen3
	if em, ok := component(status, "em", 0); ok {
		device.Capabilities.SupportsEnergyMeter = true
		device.Capabilities.SupportsPower = true
		if device.Type == "" || device.Type == "switch" {
			device.Type = "meter"
		}
		if p, ok := num(em, "total_act_power"); ok {
			device.State.TotalPower = p
			device.State.Power = p
		}
		device.State.PowerA, _ = num(em, "a_act_power")
		device.State.PowerB, _ = num(em, "b_act_power")
		device.State.PowerC, _ = num(em, "c_act_power")
		device.State.VoltageA, _ = num(em, "a_voltage")
		device.State.VoltageB, _ = num(em, "b_voltage")
		device.State.VoltageC, _ = num(em, "c_voltage")
		device.State.CurrentA, _ = num(em, "a_current")
		device.State.CurrentB, _ = num(em, "b_current")
		device.State.CurrentC, _ = num(em, "c_current")
	}
	if emdata, ok := component(status, "emdata", 0); ok {
		if total, ok := num(emdata, "total_act"); ok {
			device.State.TotalEnergy = total
			device.State.Energy = total
		}
	}

	// Single-phase energy meters (em1:N) - Pro EM, EM Gen3
	em1Idx := componentIndexes(status, "em1")
	for _, i := range em1Idx {
		em1, _ := component(status, "em1", i)
		device.Capabilities.SupportsEnergyMeter = true
		device.Capabilities.SupportsPower = true
		if device.Type == "" || device.Type == "switch" {
			device.Type = "meter"
		}
		p, _ := num(em1, "act_power")
		v, _ := num(em1, "voltage")
		a, _ := num(em1, "current")
		switch i {
		case 0:
			device.State.Power, device.State.PowerA = p, p
			device.State.Voltage, device.State.VoltageA = v, v
			device.State.Current, device.State.CurrentA = a, a
		case 1:
			device.State.PowerB, device.State.VoltageB, device.State.CurrentB = p, v, a
		case 2:
			device.State.PowerC, device.State.VoltageC, device.State.CurrentC = p, v, a
		}
	}
	if len(em1Idx) > 0 {
		// Sum of the single-phase meters; em:0 and em1:N never coexist on one device.
		var total float64
		for _, i := range componentIndexes(status, "em1data") {
			em1data, _ := component(status, "em1data", i)
			if t, ok := num(em1data, "total_act_energy"); ok {
				total += t
				if i == 0 {
					device.State.Energy = t
				}
			}
		}
		device.State.TotalEnergy = total
	}

	// Standalone power meter (pm1:0) - PM Mini Gen3
	if pm1, ok := component(status, "pm1", 0); ok {
		device.Capabilities.SupportsPower = true
		if device.Type == "" {
			device.Type = "meter"
		}
		device.State.Power, _ = num(pm1, "apower")
		device.State.Voltage, _ = num(pm1, "voltage")
		device.State.Current, _ = num(pm1, "current")
		if energy, ok := pm1["aenergy"].(map[string]interface{}); ok {
			if total, ok := num(energy, "total"); ok {
				device.State.Energy = total
			}
		}
	}

	// Determine type for sensor-only devices
	if device.Type == "" {
		caps := device.Capabilities
		switch {
		case caps.SupportsSmoke, caps.SupportsFlood, caps.SupportsTemperature, caps.SupportsHumidity:
			device.Type = "sensor"
		case caps.SupportsEnergyMeter:
			device.Type = "meter"
		case len(componentIndexes(status, "input")) > 0:
			device.Type = "input"
		}
	}
}

func parseColor(m map[string]interface{}, device *models.Device) {
	if output, ok := m["output"].(bool); ok {
		device.State.On = output
	}
	if bri, ok := num(m, "brightness"); ok {
		device.State.Brightness = bri
	}
	if rgb, ok := m["rgb"].([]interface{}); ok && len(rgb) == 3 {
		if r, ok := rgb[0].(float64); ok {
			device.State.Red = int(r)
		}
		if g, ok := rgb[1].(float64); ok {
			device.State.Green = int(g)
		}
		if b, ok := rgb[2].(float64); ok {
			device.State.Blue = int(b)
		}
	}
	// Older firmware reported separate fields.
	if r, ok := num(m, "red"); ok {
		device.State.Red = int(r)
	}
	if g, ok := num(m, "green"); ok {
		device.State.Green = int(g)
	}
	if b, ok := num(m, "blue"); ok {
		device.State.Blue = int(b)
	}
}

// parseGen2Inputs extracts input:N components.
func parseGen2Inputs(status map[string]interface{}, configID, name string) []*models.Input {
	var inputs []*models.Input
	for _, i := range componentIndexes(status, "input") {
		inp, _ := component(status, "input", i)
		input := &models.Input{
			ID:       fmt.Sprintf("%s_input_%d", configID, i),
			Name:     fmt.Sprintf("%s Input %d", name, i),
			DeviceID: configID,
			Channel:  i,
		}
		if state, ok := inp["state"].(bool); ok {
			v := state
			input.State.State = &v
		}
		inputs = append(inputs, input)
	}
	return inputs
}

// parseGen1Status maps a Gen1 /status response onto the device.
func parseGen1Status(status map[string]interface{}, device *models.Device) {
	if relays, ok := status["relays"].([]interface{}); ok && len(relays) > 0 {
		if rollers, ok := status["rollers"].([]interface{}); ok && len(rollers) > 0 {
			device.Type = "roller"
			device.Capabilities.SupportsRoller = true
			for i, r := range rollers {
				roller, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				ch := models.Channel{Index: i, Type: "cover"}
				if pos, ok := num(roller, "current_pos"); ok {
					ch.Position = int(pos)
				}
				if p, ok := num(roller, "power"); ok {
					ch.Power = p
					device.Capabilities.SupportsPower = true
				}
				if i == 0 {
					device.State.Position = ch.Position
					device.State.Power = ch.Power
				}
				device.Channels = append(device.Channels, ch)
			}
		} else {
			device.Type = "switch"
			meters, _ := status["meters"].([]interface{})
			for i, r := range relays {
				relay, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				ch := models.Channel{Index: i, Type: "switch"}
				if ison, ok := relay["ison"].(bool); ok {
					ch.On = ison
				}
				if i < len(meters) {
					if meter, ok := meters[i].(map[string]interface{}); ok {
						if p, ok := num(meter, "power"); ok {
							ch.Power = p
							device.Capabilities.SupportsPower = true
						}
						if total, ok := num(meter, "total"); ok {
							ch.Energy = total / 60 // Gen1 reports Watt-minutes
						}
					}
				}
				if i == 0 {
					device.State.On = ch.On
					device.State.Power = ch.Power
					device.State.Energy = ch.Energy
				}
				device.Channels = append(device.Channels, ch)
			}
			if len(relays) > 1 {
				device.Capabilities.MultiChannel = true
				device.Capabilities.ChannelCount = len(relays)
			}
		}
	}

	if lights, ok := status["lights"].([]interface{}); ok && len(lights) > 0 {
		device.Type = "dimmer"
		device.Capabilities.SupportsDimming = true
		if light, ok := lights[0].(map[string]interface{}); ok {
			if ison, ok := light["ison"].(bool); ok {
				device.State.On = ison
			}
			if bri, ok := num(light, "brightness"); ok {
				device.State.Brightness = bri
			}
			if mode, ok := light["mode"].(string); ok && mode == "color" {
				device.Type = "rgbw"
				device.Capabilities.SupportsColor = true
			}
			if r, ok := num(light, "red"); ok {
				device.State.Red = int(r)
				device.Type = "rgbw"
				device.Capabilities.SupportsColor = true
			}
			if g, ok := num(light, "green"); ok {
				device.State.Green = int(g)
			}
			if b, ok := num(light, "blue"); ok {
				device.State.Blue = int(b)
			}
			if w, ok := num(light, "white"); ok {
				device.State.White = int(w)
			}
			if gain, ok := num(light, "gain"); ok && device.Capabilities.SupportsColor {
				device.State.Brightness = gain
			}
		}
		if meters, ok := status["meters"].([]interface{}); ok && len(meters) > 0 {
			if meter, ok := meters[0].(map[string]interface{}); ok {
				if p, ok := num(meter, "power"); ok {
					device.State.Power = p
					device.Capabilities.SupportsPower = true
				}
			}
		}
	}

	if tmp, ok := status["tmp"].(map[string]interface{}); ok {
		if tC, ok := num(tmp, "tC"); ok {
			if valid, ok := tmp["is_valid"].(bool); !ok || valid {
				v := tC
				device.State.Temperature = &v
				device.Capabilities.SupportsTemperature = true
			}
		}
	}
	if hum, ok := status["hum"].(map[string]interface{}); ok {
		if rh, ok := num(hum, "value"); ok {
			v := rh
			device.State.Humidity = &v
			device.Capabilities.SupportsHumidity = true
		}
	}
	if bat, ok := status["bat"].(map[string]interface{}); ok {
		if pct, ok := num(bat, "value"); ok {
			v := pct
			device.State.Battery = &v
			device.Capabilities.SupportsBattery = true
		}
	}
	if device.Type == "" && (device.Capabilities.SupportsTemperature || device.Capabilities.SupportsHumidity) {
		device.Type = "sensor"
	}

	if update, ok := status["update"].(map[string]interface{}); ok {
		if oldVer, ok := update["old_version"].(string); ok {
			device.FirmwareVer = oldVer
		}
	}
}

// parseGen1Inputs extracts the inputs array of a Gen1 /status response.
func parseGen1Inputs(status map[string]interface{}, configID, name string) []*models.Input {
	var inputs []*models.Input
	inputsArr, ok := status["inputs"].([]interface{})
	if !ok {
		return inputs
	}
	for i, inp := range inputsArr {
		inpMap, ok := inp.(map[string]interface{})
		if !ok {
			continue
		}
		input := &models.Input{
			ID:       fmt.Sprintf("%s_input_%d", configID, i),
			Name:     fmt.Sprintf("%s Input %d", name, i),
			DeviceID: configID,
			Channel:  i,
		}
		if inputVal, ok := num(inpMap, "input"); ok {
			state := inputVal > 0
			input.State.State = &state
		}
		if ev, ok := inpMap["event"].(string); ok && ev != "" {
			e := ev
			input.State.Event = &e
		}
		if cnt, ok := num(inpMap, "event_cnt"); ok {
			c := int(cnt)
			input.State.EventCount = &c
		}
		inputs = append(inputs, input)
	}
	return inputs
}

// appToDeviceType maps the Gen2 "app" field to a device type. An empty result
// lets parseGen2Status decide from the reported components.
func appToDeviceType(app string) string {
	app = strings.ToLower(app)
	switch {
	case app == "":
		return ""
	case strings.Contains(app, "walldisplay"):
		return "" // has relay + sensors; type determined by components
	case strings.Contains(app, "rgbw"):
		return "rgbw"
	case strings.Contains(app, "rgb"):
		return "rgb"
	case strings.Contains(app, "dimmer"):
		return "dimmer"
	case strings.Contains(app, "light"):
		return "light"
	case strings.Contains(app, "cover"), strings.Contains(app, "roller"):
		return "roller"
	case strings.Contains(app, "smoke"), strings.Contains(app, "flood"):
		return "sensor"
	case strings.Contains(app, "blu"), strings.Contains(app, "sensor"), strings.Contains(app, "ht"):
		return "sensor"
	case strings.Contains(app, "i4"), strings.Contains(app, "input"):
		return "input"
	case strings.Contains(app, "em"), strings.Contains(app, "pm"):
		return "" // energy meter or PM variant; type determined by components
	case strings.Contains(app, "plug"), strings.Contains(app, "switch"), strings.Contains(app, "relay"):
		return "switch"
	default:
		return ""
	}
}
