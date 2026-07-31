package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func pairDevice(t *testing.T, manager *Manager) (*rsa.PrivateKey, Device) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicDER := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	public := base64.RawURLEncoding.EncodeToString(publicDER)
	session := manager.OpenPairing()
	status, err := manager.RequestPair("Test iPad", public, "192.0.2.1:1234", session.Code, "")
	if err != nil {
		t.Fatal(err)
	}
	status, err = manager.ConfirmPair(status.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Paired {
		t.Fatal("candidate was not paired")
	}
	return key, manager.ListDevices()[0]
}

func TestPairingRequiresServerInvitationAndClientConfirmation(t *testing.T) {
	manager, err := New(t.TempDir(), "server")
	if err != nil {
		t.Fatal(err)
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, err := manager.RequestPair("device", public, "192.0.2.1:1234", session.Code, "")
	if err != nil {
		t.Fatal(err)
	}
	if !status.ServerApproved || status.Paired || manager.DeviceCount() != 0 {
		t.Fatal("valid server invitation was not consumed before client confirmation")
	}
	status, _ = manager.ConfirmPair(status.ID)
	if !status.Paired || manager.DeviceCount() != 1 {
		t.Fatal("client confirmation did not finish invited pairing")
	}
}

func TestPairingRequiresExactRSA2048Key(t *testing.T) {
	manager, err := New(t.TempDir(), "server")
	if err != nil {
		t.Fatal(err)
	}
	for _, bits := range []int{1024, 4096} {
		modulus := new(big.Int).Lsh(big.NewInt(1), uint(bits-1))
		modulus.Or(modulus, big.NewInt(1))
		public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&rsa.PublicKey{N: modulus, E: 65537}))
		session := manager.OpenPairing()
		if _, err := manager.RequestPair("device", public, "192.0.2.1:1234", session.Code, ""); err == nil {
			t.Fatalf("accepted RSA-%d device key", bits)
		}
	}
}

func TestPairRequestDeduplicatesAndCanBeCancelled(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, err := manager.RequestPair("device", public, "192.0.2.1:1234", session.Code, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := manager.RequestPair("renamed device", public, "192.0.2.1:9999", session.Code, "")
	if err != nil || again.ID != status.ID || again.DeviceName != "renamed device" || len(manager.ListCandidates()) != 1 {
		t.Fatalf("duplicate request = %+v, %v", again, err)
	}
	if !manager.RejectPair(status.ID) || manager.RejectPair(status.ID) {
		t.Fatal("pairing cancellation was not single-use")
	}
	if _, ok := manager.PairStatus(status.ID); ok {
		t.Fatal("cancelled request remained visible")
	}
}

func TestChallengeSessionTicketAndRevocation(t *testing.T) {
	manager, err := New(t.TempDir(), "server")
	if err != nil {
		t.Fatal(err)
	}
	key, device := pairDevice(t, manager)
	challenge, err := manager.NewChallenge(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	digest := authDigest(manager.serverID, challenge)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, cryptoHashSHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := manager.CompleteChallenge(challenge.ID, base64.RawURLEncoding.EncodeToString(signature))
	if err != nil || deviceID != device.ID {
		t.Fatalf("complete = %q, %v", deviceID, err)
	}
	if candidates := manager.ListCandidates(); len(candidates) != 0 {
		t.Fatalf("authenticated device left %d completed pairing requests", len(candidates))
	}
	if _, err := manager.CompleteChallenge(challenge.ID, base64.RawURLEncoding.EncodeToString(signature)); err == nil {
		t.Fatal("challenge replay succeeded")
	}

	recorder := httptest.NewRecorder()
	manager.SetCookie(recorder, device.ID)
	request := httptest.NewRequest("GET", "https://surf/api/v1/config", nil)
	request.AddCookie(recorder.Result().Cookies()[0])
	if got, ok := manager.Valid(request); !ok || got != device.ID {
		t.Fatalf("valid session = %q, %v", got, ok)
	}
	ticket := manager.WSTicket(device.ID)
	if got, ok := manager.VerifyWSTicket(ticket); !ok || got != device.ID {
		t.Fatalf("ticket = %q, %v", got, ok)
	}
	if _, ok := manager.VerifyWSTicket(ticket); ok {
		t.Fatal("ticket replay succeeded")
	}
	if !manager.Revoke(device.ID) {
		t.Fatal("revoke failed")
	}
	if _, ok := manager.Valid(request); ok {
		t.Fatal("revoked session remained valid")
	}
}

func TestPairingPhraseIsDeterministic(t *testing.T) {
	public := []byte("public-key")
	first := pairingPhrase("server", public)
	if first != pairingPhrase("server", public) || first == pairingPhrase("other", public) {
		t.Fatal("pairing phrase is not identity-bound")
	}
	if sha256.Sum256([]byte(first)) == sha256.Sum256(nil) {
		t.Fatal("empty phrase")
	}
	if len(strings.Fields(first)) != 6 {
		t.Fatalf("phrase = %q, want six words", first)
	}
}

func TestPendingPairingDoesNotExpireAndChallengeDoes(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, err := manager.RequestPair("waiting device", public, "192.0.2.1:1234", session.Code, "")
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.candidates[status.ID].RequestedAt = time.Now().Add(-24 * time.Hour)
	manager.mu.Unlock()
	if current, ok := manager.PairStatus(status.ID); !ok || current.DeviceID != status.DeviceID {
		t.Fatalf("pending request expired: %+v, %v", current, ok)
	}

	_, device := pairDevice(t, manager)
	challenge, err := manager.NewChallenge(device.ID)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	value := manager.challenges[challenge.ID]
	value.ExpiresAt = time.Now().Add(-time.Second)
	manager.challenges[challenge.ID] = value
	manager.mu.Unlock()
	if _, err := manager.CompleteChallenge(challenge.ID, "anything"); err != ErrChallengeExpired {
		t.Fatalf("expired challenge = %v", err)
	}
}

func TestCompletedPairingIsRetryableUntilAuthentication(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, _ := manager.RequestPair("device", public, "192.0.2.1:1234", session.Code, "")
	status, _ = manager.ConfirmPair(status.ID)
	if !status.Paired {
		t.Fatal("request did not pair")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if completed, ok := manager.PairStatus(status.ID); !ok || !completed.Paired {
			t.Fatalf("completed retry %d = %+v, %v", attempt, completed, ok)
		}
		if confirmed, err := manager.ConfirmPair(status.ID); err != nil || !confirmed.Paired {
			t.Fatalf("confirm retry %d = %+v, %v", attempt, confirmed, err)
		}
	}
	if !manager.AckPair(status.ID) || manager.AckPair(status.ID) {
		t.Fatal("completed pairing acknowledgement was not single-use")
	}
	session = manager.OpenPairing()
	if _, err := manager.RequestPair("device", public, "192.0.2.1:1234", session.Code, ""); err != ErrAlreadyPaired {
		t.Fatalf("already-paired key request = %v", err)
	}
}

func TestCompletedPairingCannotBeCancelled(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, _ := manager.RequestPair("device", public, "192.0.2.1:1234", session.Code, "")
	_, _ = manager.ConfirmPair(status.ID)
	if manager.RejectPair(status.ID) {
		t.Fatal("completed pairing was cancelled")
	}
}

func syntheticPublicKey(index int64) string {
	modulus := new(big.Int).Lsh(big.NewInt(1), 2047)
	modulus.Add(modulus, big.NewInt(index*2+1))
	return base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&rsa.PublicKey{N: modulus, E: 65537}))
}

func TestPairingIsClosedWithoutServerInvitation(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	if _, err := manager.RequestPair("device", syntheticPublicKey(1), "192.0.2.1:1234", "123456", ""); err != ErrPairingClosed {
		t.Fatalf("closed pairing = %v", err)
	}
}

func TestPairingCodeLocksAfterFiveFailuresAndQRIsSingleUse(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	session := manager.OpenPairing()
	wrongCode := "000000"
	if session.Code == wrongCode {
		wrongCode = "999999"
	}
	for i := 0; i < MaxPairingAttempts-1; i++ {
		if _, err := manager.RequestPair("device", syntheticPublicKey(int64(i)), "192.0.2.1:1234", wrongCode, ""); err != ErrPairingCode {
			t.Fatalf("wrong code %d = %v", i+1, err)
		}
	}
	if _, err := manager.RequestPair("device", syntheticPublicKey(99), "192.0.2.1:1234", wrongCode, ""); err != ErrPairingLocked {
		t.Fatalf("fifth wrong code = %v", err)
	}
	if manager.PairingAvailable() {
		t.Fatal("locked pairing session remained open")
	}
	session = manager.OpenPairing()
	first, err := manager.RequestPair("first", syntheticPublicKey(100), "192.0.2.1:1234", "", session.QRToken)
	if err != nil || first.ID == "" {
		t.Fatalf("QR pairing = %+v, %v", first, err)
	}
	if _, err := manager.RequestPair("other", syntheticPublicKey(101), "192.0.2.2:1234", "", session.QRToken); err != ErrPairingUsed {
		t.Fatalf("reused QR = %v", err)
	}
}

func TestCompletedPairingIsEventuallyPruned(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, _ := manager.RequestPair("device", public, "192.0.2.1:1234", session.Code, "")
	_, _ = manager.ConfirmPair(status.ID)
	manager.mu.Lock()
	manager.candidates[status.ID].completedAt = time.Now().Add(-CompletedPairTTL - time.Second)
	manager.mu.Unlock()
	if _, ok := manager.PairStatus(status.ID); ok {
		t.Fatal("stale completed pairing was not pruned")
	}
}

func TestChangedClientKeyAndExpiredSessionFail(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	_, device := pairDevice(t, manager)
	challenge, _ := manager.NewChallenge(device.ID)
	wrongKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	digest := authDigest(manager.serverID, challenge)
	signature, _ := rsa.SignPKCS1v15(rand.Reader, wrongKey, cryptoHashSHA256, digest[:])
	if _, err := manager.CompleteChallenge(challenge.ID, base64.RawURLEncoding.EncodeToString(signature)); err != ErrBadSignature {
		t.Fatalf("changed client key = %v", err)
	}

	expired := "v1." + device.ID + ".1"
	expired += "." + manager.sign(expired)
	request := httptest.NewRequest("GET", "https://surf/api/v1/config", nil)
	request.AddCookie(&http.Cookie{Name: CookieName, Value: expired})
	if _, ok := manager.Valid(request); ok {
		t.Fatal("expired session was accepted")
	}
}

func TestDeviceRegistryPersistsAndRevocationCallbackRuns(t *testing.T) {
	home := t.TempDir()
	manager, _ := New(home, "server")
	_, device := pairDevice(t, manager)
	info, err := os.Stat(filepath.Join(home, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("devices.json mode = %o", info.Mode().Perm())
	}
	reloaded, err := New(home, "server")
	if err != nil || reloaded.DeviceCount() != 1 {
		t.Fatalf("reloaded devices = %d, %v", reloaded.DeviceCount(), err)
	}
	called := ""
	reloaded.SetRevokeHandler(func(id string) { called = id })
	if !reloaded.Revoke(device.ID) || called != device.ID {
		t.Fatalf("revoke callback = %q", called)
	}
	again, _ := New(home, "server")
	if again.DeviceCount() != 0 {
		t.Fatal("revoked device returned after reload")
	}
}

func TestRateLimit(t *testing.T) {
	manager, _ := New(t.TempDir(), "server")
	for i := 0; i < 10; i++ {
		if !manager.AllowPair("192.0.2.10:1234") {
			t.Fatalf("attempt %d was rejected early", i+1)
		}
	}
	if manager.AllowPair("192.0.2.10:9999") {
		t.Fatal("rate limit did not reject the eleventh attempt")
	}
	if !manager.AllowPair("192.0.2.11:1234") {
		t.Fatal("rate limit leaked across addresses")
	}
	for i := 0; i < 60; i++ {
		if !manager.AllowAuth("192.0.2.10:1234") {
			t.Fatalf("auth attempt %d was affected by pairing rate limit", i+1)
		}
	}
	if manager.AllowAuth("192.0.2.10:1234") {
		t.Fatal("auth rate limit did not reject the sixty-first attempt")
	}
}
