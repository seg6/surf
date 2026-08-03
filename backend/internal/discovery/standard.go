//go:build !windows

package discovery

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/grandcat/zeroconf"
)

type standardAdvertisement struct{ server *zeroconf.Server }

// Register publishes a Surf service using the platform multicast sockets.
func Register(name string, port int, text []string) (Advertisement, error) {
	if value := strings.TrimSpace(os.Getenv("SURF_ADVERTISE_IP")); value != "" {
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid SURF_ADVERTISE_IP %q", value)
		}
		address := ip.To4()
		if address == nil {
			address = ip.To16()
		}
		host := "surf-" + hex.EncodeToString(address) + ".local."
		server, err := zeroconf.RegisterProxy(name, "_surf._tcp", "local", port, host, []string{value}, text, nil)
		if err != nil {
			return nil, err
		}
		return &standardAdvertisement{server: server}, nil
	}
	server, err := zeroconf.Register(name, "_surf._tcp", "local", port, text, nil)
	if err != nil {
		return nil, err
	}
	return &standardAdvertisement{server: server}, nil
}

func (a *standardAdvertisement) Shutdown() { a.server.Shutdown() }
