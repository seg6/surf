//go:build !windows

package discovery

import "github.com/grandcat/zeroconf"

type standardAdvertisement struct{ server *zeroconf.Server }

// Register publishes a Surf service using the platform multicast sockets.
func Register(name string, port int, text []string) (Advertisement, error) {
	server, err := zeroconf.Register(name, "_surf._tcp", "local.", port, text, nil)
	if err != nil {
		return nil, err
	}
	return &standardAdvertisement{server: server}, nil
}

func (a *standardAdvertisement) Shutdown() { a.server.Shutdown() }
