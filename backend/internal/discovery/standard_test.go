//go:build !windows

package discovery

import "testing"

func TestRegisterRejectsInvalidExplicitAddress(t *testing.T) {
	t.Setenv("SURF_ADVERTISE_IP", "definitely-not-an-address")
	if _, err := Register("Surf", 18080, nil); err == nil {
		t.Fatal("Register accepted an invalid explicit advertisement address")
	}
}
