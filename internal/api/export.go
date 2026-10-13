package api

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/sbeyeler/loxone2shelly/internal/config"
	"github.com/sbeyeler/loxone2shelly/internal/models"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// sanitizeName converts a name to a safe identifier for Loxone
func sanitizeName(name string) string {
	s := strings.ReplaceAll(name, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = nonAlphaNum.ReplaceAllString(s, "")
	return s
}

// xmlAttr escapes a value for use inside a double-quoted XML attribute.
func xmlAttr(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func exportName(m models.Mapping) string {
	name := sanitizeName(m.Name)
	if name == "" {
		name = sanitizeName(m.ShellyID)
	}
	if m.Channel != nil {
		name = fmt.Sprintf("%s_ch%d", name, *m.Channel)
	}
	return name
}

// propertiesForType lists the UDP feedback properties a mapping type reports.
func propertiesForType(shellyType string) []string {
	switch shellyType {
	case "dimmer", "light":
		return []string{"on", "bri", "power"}
	case "rgb":
		return []string{"on", "bri", "red", "green", "blue", "power"}
	case "rgbw":
		return []string{"on", "bri", "red", "green", "blue", "white", "power"}
	case "roller", "cover":
		return []string{"on", "pos", "power"}
	case "input":
		return []string{"input", "push", "event"}
	case "sensor":
		return []string{"temp", "hum", "lux", "bat", "smoke", "flood"}
	case "meter":
		return []string{"power", "voltage", "current", "energy", "total_power", "power_a", "power_b", "power_c", "total_energy"}
	default:
		return []string{"on", "power", "voltage", "current", "energy"}
	}
}

// ExportVirtualInputs generates Loxone Virtual Input XML for Shelly feedback
func (h *Handlers) ExportVirtualInputs(w http.ResponseWriter, r *http.Request) {
	mappings := config.GetMappings()

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<VirtualInUdp Title="Loxone2Shelly Feedback">` + "\n")

	for _, m := range mappings {
		if !m.Enabled {
			continue
		}
		name := exportName(m)
		for _, prop := range propertiesForType(m.ShellyType) {
			analog := "true"
			if prop == "event" {
				analog = "false"
			}
			fmt.Fprintf(&sb, `  <VirtualInUdpCmd Title="%s_%s" Check="%s/%s:\v" Analog="%s"/>`+"\n",
				xmlAttr(name), prop, xmlAttr(m.LoxoneID), prop, analog)
		}
	}

	sb.WriteString(`</VirtualInUdp>` + "\n")

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=loxone2shelly_virtual_inputs.xml")
	if _, err := w.Write([]byte(sb.String())); err != nil {
		log.Debug().Err(err).Msg("Failed to write export")
	}
}

// ExportVirtualOutputs generates Loxone Virtual HTTP Output XML for Shelly control
func (h *Handlers) ExportVirtualOutputs(w http.ResponseWriter, r *http.Request) {
	mappings := config.GetMappings()
	cfg := config.Get()

	gatewayAddr := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	if cfg.Server.Host == "0.0.0.0" || cfg.Server.Host == "" || cfg.Server.Host == "::" {
		gatewayAddr = fmt.Sprintf("http://GATEWAY_IP:%d", cfg.Server.Port)
	}

	// Loxone sends the token as a query parameter because Virtual Outputs
	// cannot set request headers.
	tokenSuffix := ""
	if cfg.Server.APIToken != "" {
		tokenSuffix = "?token=API_TOKEN"
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	fmt.Fprintf(&sb, `<VirtualOut Title="Loxone2Shelly Control" Address="%s">`+"\n", xmlAttr(gatewayAddr))
	if tokenSuffix != "" {
		sb.WriteString("  <!-- Replace API_TOKEN with the configured server.api_token -->\n")
	}

	for _, m := range mappings {
		if !m.Enabled {
			continue
		}
		name := exportName(m)
		path := fmt.Sprintf("/api/devices/%s%s", m.ShellyID, tokenSuffix)
		channel := ""
		if m.Channel != nil {
			channel = fmt.Sprintf(`,"channel":%d`, *m.Channel)
		}

		fmt.Fprintf(&sb, `  <VirtualOutCmd Title="%s ON" CmdOnMethod="PUT" CmdOn="%s" CmdOnBody='{"on":true%s}'/>`+"\n",
			xmlAttr(name), xmlAttr(path), channel)
		fmt.Fprintf(&sb, `  <VirtualOutCmd Title="%s OFF" CmdOnMethod="PUT" CmdOn="%s" CmdOnBody='{"on":false%s}'/>`+"\n",
			xmlAttr(name), xmlAttr(path), channel)

		fmt.Fprintf(&sb, "  <!-- WebSocket: SET %s ON | SET %s OFF -->\n", xmlAttr(m.LoxoneID), xmlAttr(m.LoxoneID))

		switch m.ShellyType {
		case "dimmer", "light", "rgb", "rgbw":
			fmt.Fprintf(&sb, `  <VirtualOutCmd Title="%s BRI" CmdOnMethod="PUT" CmdOn="%s" CmdOnBody='{"on":true,"brightness":<v>%s}'/>`+"\n",
				xmlAttr(name), xmlAttr(path), channel)
			fmt.Fprintf(&sb, "  <!-- WebSocket: SET %s BRI <value> -->\n", xmlAttr(m.LoxoneID))
		case "roller", "cover":
			fmt.Fprintf(&sb, `  <VirtualOutCmd Title="%s POS" CmdOnMethod="PUT" CmdOn="%s" CmdOnBody='{"position":<v>%s}'/>`+"\n",
				xmlAttr(name), xmlAttr(path), channel)
			fmt.Fprintf(&sb, "  <!-- WebSocket: SET %s POS <value> -->\n", xmlAttr(m.LoxoneID))
		}
	}

	sb.WriteString(`</VirtualOut>` + "\n")

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=loxone2shelly_virtual_outputs.xml")
	if _, err := w.Write([]byte(sb.String())); err != nil {
		log.Debug().Err(err).Msg("Failed to write export")
	}
}
