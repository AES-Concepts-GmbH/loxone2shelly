package shelly

import (
	"time"

	"github.com/rs/zerolog/log"
)

// DiscoveredDevice represents a Shelly device found via discovery
type DiscoveredDevice struct {
	ID   string `json:"id"`
	IP   string `json:"ip"`
	Name string `json:"name"`
	Gen  int    `json:"gen"`
}

// DiscoverDevices attempts to discover Shelly devices on the local network.
//
// Note: Shelly Gen2 devices register themselves via mDNS under "_shelly._tcp".
// Since we do not have a zeroconf dependency, automatic mDNS discovery is not
// available. This function returns an empty list and logs a hint that devices
// must be added manually via the configuration or the API.
//
// In the future, a zeroconf library could be added to go.mod to enable
// automatic discovery.
func DiscoverDevices(timeout time.Duration) ([]DiscoveredDevice, error) {
	_ = timeout // reserved for future mDNS implementation

	log.Info().Msg("Automatic Shelly device discovery is not available (no mDNS/zeroconf dependency). " +
		"Please add devices manually via the configuration file or the REST API.")

	return []DiscoveredDevice{}, nil
}
