package loxone

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

// Sender is implemented by all Loxone feedback transports.
type Sender interface {
	IsEnabled() bool
	Send(miniserverID, loxoneID, property string, value interface{})
	SendToAll(loxoneID, property string, value interface{})
}

// Dispatcher routes Shelly state changes to the Loxone Miniservers according to
// the configured mappings. It is the single place that knows which properties
// a device type reports.
type Dispatcher struct {
	mappings *MappingManager
	udp      Sender
	http     Sender
	mqtt     Sender
}

// NewDispatcher creates a feedback dispatcher. Any sender may be nil.
func NewDispatcher(mappings *MappingManager, udp, http, mqtt Sender) *Dispatcher {
	return &Dispatcher{mappings: mappings, udp: udp, http: http, mqtt: mqtt}
}

// DispatchDevice sends the full state of a device (or one of its channels) to
// every mapping that targets it.
func (d *Dispatcher) DispatchDevice(device *models.Device, channel *int) {
	if device == nil {
		return
	}
	d.DispatchProps(device.ID, channel, PropsFromDevice(device, channel))
}

// DispatchProps sends explicit properties for a device/channel to every mapping
// that targets it.
func (d *Dispatcher) DispatchProps(deviceID string, channel *int, props []models.Prop) {
	if len(props) == 0 {
		return
	}
	for _, mapping := range d.mappings.GetForChannel(deviceID, channel) {
		d.SendToMapping(&mapping, mapping.LoxoneID, props)
	}
}

// SendToMapping sends properties for a mapping. loxoneID may differ from the
// mapping's own ID (e.g. with a channel suffix for webhook events).
func (d *Dispatcher) SendToMapping(mapping *models.Mapping, loxoneID string, props []models.Prop) {
	msID := mapping.MiniserverID

	send := func(s Sender, enabled bool, name string) {
		if s == nil || !enabled || !s.IsEnabled() {
			return
		}
		for _, p := range props {
			if msID != "" {
				s.Send(msID, loxoneID, p.Name, p.Value)
			} else {
				s.SendToAll(loxoneID, p.Name, p.Value)
			}
		}
		log.Debug().Str("transport", name).Str("loxone_id", loxoneID).Int("props", len(props)).Msg("Feedback dispatched")
	}

	send(d.udp, mapping.ShouldFeedbackUDP(), "udp")
	send(d.http, mapping.ShouldFeedbackHTTP(), "http")
	send(d.mqtt, mapping.ShouldFeedbackMQTT(), "mqtt")
}

// PropsFromDevice derives the Loxone feedback properties for a device. With a
// channel set, the properties of that channel are used instead of the primary
// state. Values are always sent, including zero, so Loxone sees "off" and
// "0 W" transitions.
func PropsFromDevice(device *models.Device, channel *int) []models.Prop {
	caps := device.Capabilities
	var props []models.Prop
	add := func(name string, value interface{}) {
		props = append(props, models.Prop{Name: name, Value: value})
	}

	if channel != nil {
		var ch *models.Channel
		for i := range device.Channels {
			if device.Channels[i].Index == *channel {
				ch = &device.Channels[i]
				break
			}
		}
		if ch == nil {
			return nil
		}
		add("on", boolToInt(ch.On))
		switch ch.Type {
		case "light":
			add("bri", int(ch.Brightness))
		case "cover":
			add("pos", ch.Position)
		}
		if caps.SupportsPower {
			add("power", fmt.Sprintf("%.1f", ch.Power))
			add("voltage", fmt.Sprintf("%.1f", ch.Voltage))
			add("current", fmt.Sprintf("%.3f", ch.Current))
			add("energy", fmt.Sprintf("%.1f", ch.Energy))
		}
		return props
	}

	state := device.State
	add("on", boolToInt(state.On))

	if caps.SupportsDimming {
		add("bri", int(state.Brightness))
	}
	if caps.SupportsColorTemp {
		add("ct", state.ColorTemp)
	}
	if caps.SupportsColor {
		add("red", state.Red)
		add("green", state.Green)
		add("blue", state.Blue)
		if device.Type == "rgbw" {
			add("white", state.White)
		}
	}
	if caps.SupportsRoller {
		add("pos", state.Position)
	}
	if caps.SupportsPower {
		add("power", fmt.Sprintf("%.1f", state.Power))
		add("voltage", fmt.Sprintf("%.1f", state.Voltage))
		add("current", fmt.Sprintf("%.3f", state.Current))
		add("energy", fmt.Sprintf("%.1f", state.Energy))
	}
	if state.Temperature != nil {
		add("temp", fmt.Sprintf("%.1f", *state.Temperature))
	}
	if state.Humidity != nil {
		add("hum", fmt.Sprintf("%.1f", *state.Humidity))
	}
	if state.Illuminance != nil {
		add("lux", fmt.Sprintf("%.0f", *state.Illuminance))
	}
	if state.Battery != nil {
		add("bat", fmt.Sprintf("%.0f", *state.Battery))
	}
	if state.Smoke != nil {
		add("smoke", boolToInt(*state.Smoke))
	}
	if state.Flood != nil {
		add("flood", boolToInt(*state.Flood))
	}
	if caps.SupportsEnergyMeter {
		add("total_power", fmt.Sprintf("%.1f", state.TotalPower))
		add("power_a", fmt.Sprintf("%.1f", state.PowerA))
		add("power_b", fmt.Sprintf("%.1f", state.PowerB))
		add("power_c", fmt.Sprintf("%.1f", state.PowerC))
		add("voltage_a", fmt.Sprintf("%.1f", state.VoltageA))
		add("voltage_b", fmt.Sprintf("%.1f", state.VoltageB))
		add("voltage_c", fmt.Sprintf("%.1f", state.VoltageC))
		add("current_a", fmt.Sprintf("%.3f", state.CurrentA))
		add("current_b", fmt.Sprintf("%.3f", state.CurrentB))
		add("current_c", fmt.Sprintf("%.3f", state.CurrentC))
		add("total_energy", fmt.Sprintf("%.1f", state.TotalEnergy))
	}

	return props
}

// WebhookProps converts a Shelly action-URL event into feedback properties.
// Button events are reported on "push" / "btn", relay events on "on", so they
// never collide with the relay state of the same mapping.
func WebhookProps(event, value string) []models.Prop {
	props := []models.Prop{{Name: "event", Value: event}}
	switch event {
	case "on":
		props = append(props, models.Prop{Name: "on", Value: 1})
	case "off":
		props = append(props, models.Prop{Name: "on", Value: 0})
	case "toggle":
		props = append(props, models.Prop{Name: "toggle", Value: 1})
	case "single_push":
		props = append(props, models.Prop{Name: "push", Value: 1})
	case "double_push":
		props = append(props, models.Prop{Name: "push", Value: 2})
	case "triple_push":
		props = append(props, models.Prop{Name: "push", Value: 4})
	case "long_push":
		props = append(props, models.Prop{Name: "push", Value: 3})
	case "btn_down":
		props = append(props, models.Prop{Name: "btn", Value: 1})
	case "btn_up":
		props = append(props, models.Prop{Name: "btn", Value: 0})
	}
	if value != "" {
		props = append(props, models.Prop{Name: "value", Value: value})
	}
	return props
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
