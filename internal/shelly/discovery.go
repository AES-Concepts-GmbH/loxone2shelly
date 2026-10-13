package shelly

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/rs/zerolog/log"
)

// DiscoveredDevice represents a Shelly device found via mDNS
type DiscoveredDevice struct {
	ID    string `json:"id"`
	IP    string `json:"ip"`
	Name  string `json:"name"`
	Gen   int    `json:"gen"`
	Model string `json:"model,omitempty"`
}

// DiscoverDevices browses the local network via mDNS for Shelly devices.
// Gen2+ devices announce "_shelly._tcp"; Gen1 devices announce "_http._tcp"
// with a "shelly..." instance name. The container needs host networking
// (or multicast access) for mDNS to work.
func DiscoverDevices(ctx context.Context, timeout time.Duration) ([]DiscoveredDevice, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		mu    sync.Mutex
		found = make(map[string]DiscoveredDevice)
		wg    sync.WaitGroup
	)

	for _, service := range []string{"_shelly._tcp", "_http._tcp"} {
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			return nil, err
		}
		// zeroconf closes the entries channel when ctx ends, so each browse
		// gets its own channel.
		entries := make(chan *zeroconf.ServiceEntry, 64)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range entries {
				dev, ok := entryToDevice(e)
				if !ok {
					continue
				}
				mu.Lock()
				if cur, exists := found[dev.IP]; !exists || dev.Gen > cur.Gen {
					found[dev.IP] = dev
				}
				mu.Unlock()
			}
		}()
		if err := resolver.Browse(ctx, service, "local.", entries); err != nil {
			log.Warn().Err(err).Str("service", service).Msg("mDNS browse failed")
			close(entries)
		}
	}

	<-ctx.Done()
	wg.Wait()

	result := make([]DiscoveredDevice, 0, len(found))
	for _, d := range found {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

	if len(result) == 0 {
		log.Info().Msg("mDNS discovery found no Shelly devices (needs host networking / multicast)")
	}
	return result, nil
}

func entryToDevice(e *zeroconf.ServiceEntry) (DiscoveredDevice, bool) {
	name := strings.ToLower(e.Instance)
	isShelly := strings.HasSuffix(e.Service, "_shelly._tcp") || strings.HasPrefix(name, "shelly")
	if !isShelly || len(e.AddrIPv4) == 0 {
		return DiscoveredDevice{}, false
	}

	dev := DiscoveredDevice{
		ID:   name,
		Name: e.Instance,
		IP:   e.AddrIPv4[0].String(),
		Gen:  1,
	}
	if strings.HasSuffix(e.Service, "_shelly._tcp") {
		dev.Gen = 2
	}
	for _, txt := range e.Text {
		kv := strings.SplitN(txt, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.ToLower(kv[0]) {
		case "gen":
			switch kv[1] {
			case "1":
				dev.Gen = 1
			case "2", "3", "4":
				dev.Gen = 2
			}
		case "app", "model":
			dev.Model = kv[1]
		}
	}
	return dev, true
}
