// Package discovery advertises Surf through DNS Service Discovery.
package discovery

// Advertisement is a live Bonjour registration.
type Advertisement interface {
	Shutdown()
}
