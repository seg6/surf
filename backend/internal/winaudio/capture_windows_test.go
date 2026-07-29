//go:build windows

package winaudio

import (
	"testing"
	"unsafe"
)

func TestWindowsABIStructLayouts(t *testing.T) {
	if got := unsafe.Sizeof(audioClientActivationParams{}); got != 12 {
		t.Fatalf("AUDIOCLIENT_ACTIVATION_PARAMS size=%d, want 12", got)
	}
	if got := unsafe.Sizeof(propVariant{}); got != 24 {
		t.Fatalf("PROPVARIANT size=%d, want 24", got)
	}
	if got := unsafe.Sizeof(waveFormat{}); got != 20 {
		t.Fatalf("WAVEFORMATEX Go storage size=%d, want 20", got)
	}
}

func TestHRESULTFailureClassification(t *testing.T) {
	if failed(sOK) {
		t.Fatal("S_OK classified as failure")
	}
	if !failed(eNoInterface) {
		t.Fatal("E_NOINTERFACE classified as success")
	}
}
