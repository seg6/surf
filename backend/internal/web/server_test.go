package web

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"surf-backend/internal/auth"
	"surf-backend/internal/config"
	"surf-backend/internal/identity"
	"surf-backend/internal/transport"
)

const testAdminToken = "test-admin-token"

func newTestServer(t *testing.T) (*Server, *auth.Manager) {
	t.Helper()
	home := t.TempDir()
	ident, err := identity.LoadOrCreate(home, "Test Surf")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := auth.New(home, ident.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ServerName: "Test Surf", ViewW: 768, ViewH: 934}
	server := New(cfg, manager, ident, transport.New())
	server.SetAdminToken(testAdminToken)
	return server, manager
}

func adminRequest(method, path string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Surf-Admin", testAdminToken)
	return request
}

func adminJSONRequest(method, path string, body any) *http.Request {
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Surf-Admin", testAdminToken)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestDirectTLSRejectsPlaintextDowngradeAndChangedIdentity(t *testing.T) {
	server, _ := newTestServer(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { _ = ServeTLS(listener, server.identity, server.Handler()) }()
	address := listener.Addr().String()

	plainClient := &http.Client{Timeout: 2 * time.Second}
	response, plainErr := plainClient.Get("http://" + address + APIRoot + "/health")
	if plainErr == nil {
		response.Body.Close()
		if response.StatusCode == http.StatusOK {
			t.Fatal("plaintext reached the Surf API")
		}
	}

	if connection, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS11}); err == nil {
		connection.Close()
		t.Fatal("TLS 1.1 downgrade succeeded")
	}

	want := server.identity.Fingerprint
	pinned := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			sum := sha256.Sum256(state.PeerCertificates[0].Raw)
			if hex.EncodeToString(sum[:]) != want {
				return fmt.Errorf("pin mismatch")
			}
			return nil
		},
	}}}
	secureResponse, err := pinned.Get("https://" + address + APIRoot + "/health")
	if err != nil || secureResponse.StatusCode != http.StatusOK {
		t.Fatalf("pinned TLS = %v, %v", secureResponse, err)
	}
	secureResponse.Body.Close()

	changed := pinned.Transport.(*http.Transport).Clone()
	changed.TLSClientConfig = changed.TLSClientConfig.Clone()
	changed.TLSClientConfig.VerifyConnection = func(tls.ConnectionState) error { return fmt.Errorf("substituted certificate") }
	if response, err := (&http.Client{Timeout: 2 * time.Second, Transport: changed}).Get("https://" + address + APIRoot + "/health"); err == nil {
		response.Body.Close()
		t.Fatal("substituted certificate was accepted")
	}

	config := TLSConfig(server.identity)
	if config.MinVersion != tls.VersionTLS12 || !config.SessionTicketsDisabled {
		t.Fatalf("TLS config min=%x ticketsDisabled=%v", config.MinVersion, config.SessionTicketsDisabled)
	}
}

func TestPairingLinkBindsAddressAndIdentity(t *testing.T) {
	link := pairingLink("https://[2001:db8::1]:18080", "server-id", "one-time-token")
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "surf" || parsed.Host != "pair" {
		t.Fatalf("pairing link = %q, %v", link, err)
	}
	if parsed.Query().Get("h") != "[2001:db8::1]:18080" || parsed.Query().Get("i") != "server-id" || parsed.Query().Get("t") != "one-time-token" {
		t.Fatalf("pairing query = %v", parsed.Query())
	}
}

func pairedCookie(t *testing.T, manager *auth.Manager) *http.Cookie {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, err := manager.RequestPair("Test iPad", public, "192.0.2.1:1234", session.Code, "")
	if err != nil {
		t.Fatal(err)
	}
	status, err = manager.ConfirmPair(status.ID)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	manager.SetCookie(recorder, status.DeviceID)
	return recorder.Result().Cookies()[0]
}

func TestOnlyVersionedRoutesExist(t *testing.T) {
	server, _ := newTestServer(t)
	for _, path := range []string{"/health", "/login", "/logout", "/native-config", "/ws", "/upload", "/downloads/file", "/tabicon/1", "/api/v1x/health"} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s = %d, want 404", path, recorder.Code)
		}
	}
	for _, path := range []string{APIRoot + "/health", APIRoot + "/server"} {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s = %d", path, recorder.Code)
		}
	}
}

func TestSignedAuthenticationEndpointsIssueUsableSession(t *testing.T) {
	server, manager := newTestServer(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	session := manager.OpenPairing()
	status, err := manager.RequestPair("iPad", public, "192.0.2.1:1234", session.Code, "")
	if err != nil {
		t.Fatal(err)
	}
	status, _ = manager.ConfirmPair(status.ID)

	payload, _ := json.Marshal(map[string]string{"deviceID": status.DeviceID})
	challengeResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(challengeResponse, httptest.NewRequest(http.MethodPost, APIRoot+"/auth/challenge", bytes.NewReader(payload)))
	if challengeResponse.Code != http.StatusOK {
		t.Fatalf("challenge = %d: %s", challengeResponse.Code, challengeResponse.Body.String())
	}
	var challenge auth.Challenge
	_ = json.Unmarshal(challengeResponse.Body.Bytes(), &challenge)
	value := "SURF-AUTH-V1\x00" + manager.ServerID() + "\x00" + status.DeviceID + "\x00" + challenge.ID + "\x00" + challenge.Nonce
	digest := sha256.Sum256([]byte(value))
	signature, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	completePayload, _ := json.Marshal(map[string]string{"challengeID": challenge.ID, "signature": base64.RawURLEncoding.EncodeToString(signature)})
	complete := httptest.NewRecorder()
	server.Handler().ServeHTTP(complete, httptest.NewRequest(http.MethodPost, APIRoot+"/auth/complete", bytes.NewReader(completePayload)))
	if complete.Code != http.StatusNoContent || len(complete.Result().Cookies()) != 1 {
		t.Fatalf("complete = %d cookies=%d: %s", complete.Code, len(complete.Result().Cookies()), complete.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, APIRoot+"/config?nv="+config.NativeVersion, nil)
	request.AddCookie(complete.Result().Cookies()[0])
	configured := httptest.NewRecorder()
	server.Handler().ServeHTTP(configured, request)
	if configured.Code != http.StatusOK {
		t.Fatalf("authenticated config = %d: %s", configured.Code, configured.Body.String())
	}
}

func TestPairedDeviceCanRevokeItself(t *testing.T) {
	server, manager := newTestServer(t)
	cookie := pairedCookie(t, manager)
	request := httptest.NewRequest(http.MethodPost, APIRoot+"/auth/revoke", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || manager.DeviceCount() != 0 {
		t.Fatalf("self revoke = %d devices=%d: %s", response.Code, manager.DeviceCount(), response.Body.String())
	}
	probe := httptest.NewRequest(http.MethodGet, APIRoot+"/config", nil)
	probe.AddCookie(cookie)
	if _, ok := manager.Valid(probe); ok {
		t.Fatal("self-revoked device session remained valid")
	}
}

func TestAdminRoutesAreLoopbackOnly(t *testing.T) {
	server, _ := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, APIRoot+"/admin/devices", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Surf-Admin", testAdminToken)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote admin = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, APIRoot+"/admin/devices", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tokenless local admin = %d", response.Code)
	}
}

func TestLoopbackStatsRequireAdminToken(t *testing.T) {
	server, _ := newTestServer(t)
	server.SetStats(func() map[string]any { return map[string]any{"fps": 30} })
	request := httptest.NewRequest(http.MethodGet, APIRoot+"/health?stats=1", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("tokenless loopback stats = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, APIRoot+"/health?stats=1", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Surf-Admin", testAdminToken)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized loopback stats = %d: %s", response.Code, response.Body.String())
	}
}

func TestPairingAndAuthenticatedConfig(t *testing.T) {
	server, manager := newTestServer(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	closedPayload, _ := json.Marshal(map[string]string{
		"deviceName": "iPad", "publicKey": base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey)),
		"code": "123456",
	})
	closed := httptest.NewRecorder()
	server.Handler().ServeHTTP(closed, httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/request", bytes.NewReader(closedPayload)))
	if closed.Code != http.StatusForbidden {
		t.Fatalf("closed pairing request = %d: %s", closed.Code, closed.Body.String())
	}
	session := manager.OpenPairing()
	payload, _ := json.Marshal(map[string]string{
		"deviceName": "iPad", "publicKey": base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey)),
		"code": session.Code,
	})
	request := httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/request", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("pair request = %d: %s", recorder.Code, recorder.Body.String())
	}
	var status auth.PairingStatus
	_ = json.Unmarshal(recorder.Body.Bytes(), &status)
	if !status.ServerApproved || status.Paired {
		t.Fatalf("invited status = %+v", status)
	}
	challengePayload, _ := json.Marshal(map[string]string{"deviceID": status.DeviceID})
	challenge := httptest.NewRecorder()
	server.Handler().ServeHTTP(challenge, httptest.NewRequest(http.MethodPost, APIRoot+"/auth/challenge", bytes.NewReader(challengePayload)))
	if challenge.Code != http.StatusUnauthorized {
		t.Fatalf("unconfirmed device received an authentication challenge: %d", challenge.Code)
	}

	confirm := httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/confirm/"+status.ID, nil)
	confirmed := httptest.NewRecorder()
	server.Handler().ServeHTTP(confirmed, confirm)
	_ = json.Unmarshal(confirmed.Body.Bytes(), &status)
	if confirmed.Code != http.StatusOK || !status.Paired || manager.DeviceCount() != 1 {
		t.Fatalf("client confirmation did not finish invited pairing: code=%d status=%+v", confirmed.Code, status)
	}

	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, APIRoot+"/config", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("config without cookie = %d", unauthorized.Code)
	}

	cookie := pairedCookie(t, manager)
	configRequest := httptest.NewRequest(http.MethodGet, APIRoot+"/config?av=0.10.0&nv="+config.NativeVersion, nil)
	configRequest.AddCookie(cookie)
	configured := httptest.NewRecorder()
	server.Handler().ServeHTTP(configured, configRequest)
	if configured.Code != http.StatusOK {
		t.Fatalf("config = %d: %s", configured.Code, configured.Body.String())
	}
	var body struct {
		Ticket string `json:"ticket"`
	}
	_ = json.Unmarshal(configured.Body.Bytes(), &body)
	if body.Ticket == "" {
		t.Fatal("config omitted websocket ticket")
	}
}

func TestServerInitiatedPairingSessionCancelAndAck(t *testing.T) {
	server, _ := newTestServer(t)
	closed := httptest.NewRecorder()
	server.Handler().ServeHTTP(closed, adminRequest(http.MethodGet, APIRoot+"/admin/pairing/session"))
	var closedBody map[string]any
	_ = json.Unmarshal(closed.Body.Bytes(), &closedBody)
	if closed.Code != http.StatusOK || closedBody["active"] != false {
		t.Fatalf("closed session = %d %v", closed.Code, closedBody)
	}

	opened := httptest.NewRecorder()
	server.Handler().ServeHTTP(opened, adminJSONRequest(http.MethodPost, APIRoot+"/admin/pairing/session", map[string]string{"publicAddress": "192.0.2.10:18080"}))
	var invitation struct {
		Active     bool   `json:"active"`
		Code       string `json:"code"`
		PairingURL string `json:"pairingURL"`
	}
	_ = json.Unmarshal(opened.Body.Bytes(), &invitation)
	parsed, _ := url.Parse(invitation.PairingURL)
	if opened.Code != http.StatusCreated || !invitation.Active || len(invitation.Code) != 6 || parsed.Query().Get("h") != "192.0.2.10:18080" || parsed.Query().Get("t") == "" {
		t.Fatalf("opened session = %d %+v", opened.Code, invitation)
	}

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	payload, _ := json.Marshal(map[string]string{
		"deviceName": "Complete me", "publicKey": base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey)), "code": invitation.Code,
	})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/request", bytes.NewReader(payload)))
	var completed auth.PairingStatus
	_ = json.Unmarshal(response.Body.Bytes(), &completed)
	if response.Code != http.StatusCreated || !completed.ServerApproved {
		t.Fatalf("invited pair request = %d %+v: %s", response.Code, completed, response.Body.String())
	}

	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	reusedPayload, _ := json.Marshal(map[string]string{
		"deviceName": "Other", "publicKey": base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&otherKey.PublicKey)), "code": invitation.Code,
	})
	reused := httptest.NewRecorder()
	server.Handler().ServeHTTP(reused, httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/request", bytes.NewReader(reusedPayload)))
	if reused.Code != http.StatusForbidden {
		t.Fatalf("reused invitation = %d: %s", reused.Code, reused.Body.String())
	}

	confirm := httptest.NewRecorder()
	server.Handler().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/confirm/"+completed.ID, nil))
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm = %d: %s", confirm.Code, confirm.Body.String())
	}
	ack := httptest.NewRecorder()
	server.Handler().ServeHTTP(ack, httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/ack/"+completed.ID, nil))
	if ack.Code != http.StatusNoContent {
		t.Fatalf("ack = %d: %s", ack.Code, ack.Body.String())
	}
	finished := httptest.NewRecorder()
	server.Handler().ServeHTTP(finished, adminRequest(http.MethodGet, APIRoot+"/admin/pairing/session"))
	var finishedBody map[string]any
	_ = json.Unmarshal(finished.Body.Bytes(), &finishedBody)
	if finishedBody["active"] != false {
		t.Fatalf("finished invitation remained active: %v", finishedBody)
	}

	opened = httptest.NewRecorder()
	server.Handler().ServeHTTP(opened, adminJSONRequest(http.MethodPost, APIRoot+"/admin/pairing/session", map[string]string{}))
	cancel := httptest.NewRecorder()
	server.Handler().ServeHTTP(cancel, adminRequest(http.MethodDelete, APIRoot+"/admin/pairing/session"))
	if cancel.Code != http.StatusNoContent {
		t.Fatalf("cancel session = %d", cancel.Code)
	}
	for _, removed := range []string{APIRoot + "/admin/pairing/info", APIRoot + "/admin/pairing/approve/anything"} {
		gone := httptest.NewRecorder()
		server.Handler().ServeHTTP(gone, adminRequest(http.MethodGet, removed))
		if gone.Code != http.StatusNotFound {
			t.Fatalf("removed route %s = %d", removed, gone.Code)
		}
	}
}

func TestPairRequestRejectsRemovedInvitationField(t *testing.T) {
	server, _ := newTestServer(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	payload, _ := json.Marshal(map[string]string{
		"deviceName": "Old client",
		"publicKey":  base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey)),
		"invitation": "removed",
	})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/request", bytes.NewReader(payload)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("old invitation field = %d, want 400", response.Code)
	}
}

func TestFifthWrongPairingCodeMapsToTooManyRequests(t *testing.T) {
	server, manager := newTestServer(t)
	remote := "192.0.2.44:1234"
	manager.OpenPairing()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	payload, _ := json.Marshal(map[string]string{
		"deviceName": "One too many",
		"publicKey":  base64.RawURLEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey)),
		"code":       "wrong",
	})
	for attempt := 1; attempt <= auth.MaxPairingAttempts; attempt++ {
		request := httptest.NewRequest(http.MethodPost, APIRoot+"/pairing/request", bytes.NewReader(payload))
		request.RemoteAddr = remote
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		want := http.StatusForbidden
		if attempt == auth.MaxPairingAttempts {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("wrong code attempt %d = %d, want %d: %s", attempt, response.Code, want, response.Body.String())
		}
	}
}

func TestClientPackageRouteIsAuthenticatedAndVersioned(t *testing.T) {
	server, manager := newTestServer(t)
	server.setClientPackage(&clientPackage{Version: "0.10.0", Protocol: config.NativeVersion, Data: []byte("!<arch>\npackage")})
	request := httptest.NewRequest(http.MethodGet, APIRoot+"/updates/client", nil)
	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized update = %d", unauthorized.Code)
	}
	request = httptest.NewRequest(http.MethodGet, APIRoot+"/updates/client", nil)
	request.AddCookie(pairedCookie(t, manager))
	authorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized update = %d", authorized.Code)
	}
}
