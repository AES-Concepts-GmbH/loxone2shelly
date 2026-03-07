package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/sbeyeler/loxone2shelly/internal/config"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// sanitizeName converts a name to a safe identifier for Loxone
func sanitizeName(name string) string {
	s := strings.ReplaceAll(name, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = nonAlphaNum.ReplaceAllString(s, "")
	return s
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

		name := sanitizeName(m.Name)
		if name == "" {
			name = sanitizeName(m.ShellyID)
		}

		// on (0/1) - always present
		sb.WriteString(fmt.Sprintf(`  <VirtualInUdpCmd Title="%s_on" Check="%s/on:\v" Analog="true"/>`,
			name, m.LoxoneID))
		sb.WriteString("\n")

		// Type-specific properties
		switch m.ShellyType {
		case "dimmer", "light", "rgb", "rgbw":
			// bri (0-100)
			sb.WriteString(fmt.Sprintf(`  <VirtualInUdpCmd Title="%s_bri" Check="%s/bri:\v" Analog="true"/>`,
				name, m.LoxoneID))
			sb.WriteString("\n")
		case "roller", "cover":
			// pos (0-100)
			sb.WriteString(fmt.Sprintf(`  <VirtualInUdpCmd Title="%s_pos" Check="%s/pos:\v" Analog="true"/>`,
				name, m.LoxoneID))
			sb.WriteString("\n")
		}

		// power - for all types (if available)
		sb.WriteString(fmt.Sprintf(`  <VirtualInUdpCmd Title="%s_power" Check="%s/power:\v" Analog="true"/>`,
			name, m.LoxoneID))
		sb.WriteString("\n")
	}

	sb.WriteString(`</VirtualInUdp>` + "\n")

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=loxone2shelly_virtual_inputs.xml")
	w.Write([]byte(sb.String()))
}

// ExportVirtualOutputs generates Loxone Virtual HTTP Output XML for Shelly control
func (h *Handlers) ExportVirtualOutputs(w http.ResponseWriter, r *http.Request) {
	mappings := config.GetMappings()
	cfg := config.Get()

	// Determine the gateway address
	gatewayAddr := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	if cfg.Server.Host == "0.0.0.0" {
		gatewayAddr = fmt.Sprintf("http://GATEWAY_IP:%d", cfg.Server.Port)
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<VirtualOut Title="Loxone2Shelly Control" Address="` + gatewayAddr + `">` + "\n")

	for _, m := range mappings {
		if !m.Enabled {
			continue
		}

		name := sanitizeName(m.Name)
		if name == "" {
			name = sanitizeName(m.ShellyID)
		}

		// ON command
		sb.WriteString(fmt.Sprintf(`  <VirtualOutCmd Title="%s ON" CmdOnMethod="PUT" CmdOn="/api/devices/%s" CmdOnBody='{"on":true}'/>`,
			name, m.ShellyID))
		sb.WriteString("\n")

		// OFF command
		sb.WriteString(fmt.Sprintf(`  <VirtualOutCmd Title="%s OFF" CmdOnMethod="PUT" CmdOn="/api/devices/%s" CmdOnBody='{"on":false}'/>`,
			name, m.ShellyID))
		sb.WriteString("\n")

		// SET via WebSocket text command
		sb.WriteString(fmt.Sprintf(`  <!-- WebSocket commands for %s: -->`, name))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`  <!-- SET %s ON -->`, m.LoxoneID))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`  <!-- SET %s OFF -->`, m.LoxoneID))
		sb.WriteString("\n")

		switch m.ShellyType {
		case "dimmer", "light", "rgb", "rgbw":
			// BRI command
			sb.WriteString(fmt.Sprintf(`  <VirtualOutCmd Title="%s BRI" CmdOnMethod="PUT" CmdOn="/api/devices/%s" CmdOnBody='{"on":true,"brightness":<v>}'/>`,
				name, m.ShellyID))
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf(`  <!-- SET %s BRI <value> -->`, m.LoxoneID))
			sb.WriteString("\n")
		case "roller", "cover":
			// POS command
			sb.WriteString(fmt.Sprintf(`  <VirtualOutCmd Title="%s POS" CmdOnMethod="PUT" CmdOn="/api/devices/%s" CmdOnBody='{"position":<v>}'/>`,
				name, m.ShellyID))
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf(`  <!-- SET %s POS <value> -->`, m.LoxoneID))
			sb.WriteString("\n")
		}
	}

	sb.WriteString(`</VirtualOut>` + "\n")

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=loxone2shelly_virtual_outputs.xml")
	w.Write([]byte(sb.String()))
}
