package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"surf-backend/internal/auth"
	"surf-backend/internal/clientupdate"
	"surf-backend/internal/config"
)

func TestHealthStatsRequiresAuth(t *testing.T) {
	a, err := auth.New(t.TempDir(), "test-password", 1)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(&config.Config{}, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetStats(func() map[string]any { return map[string]any{"activeURL": "https://example.com"} })

	public := httptest.NewRecorder()
	srv.Handler().ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/health", nil))
	if public.Code != http.StatusOK {
		t.Fatalf("public health code = %d, want %d", public.Code, http.StatusOK)
	}

	unauth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/health?stats=1", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth stats code = %d, want %d", unauth.Code, http.StatusUnauthorized)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/health?stats=1", nil)
	w := httptest.NewRecorder()
	a.SetCookie(w)
	authReq.AddCookie(w.Result().Cookies()[0])
	authResp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authResp, authReq)
	if authResp.Code != http.StatusOK {
		t.Fatalf("auth stats code = %d, want %d", authResp.Code, http.StatusOK)
	}
}

func TestNativeConfigOffersNewerEmbeddedClientOnProtocolMismatch(t *testing.T) {
	a, err := auth.New(t.TempDir(), "test-password", 1)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(&config.Config{}, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetClientUpdate(&clientupdate.Bundle{
		Version: "1.0.0", Protocol: config.NativeVersion, SHA256: "abc", Data: []byte("package"),
	})
	request := httptest.NewRequest(http.MethodGet, "/native-config?av=0.1.0&nv=old", nil)
	cookies := httptest.NewRecorder()
	a.SetCookie(cookies)
	request.AddCookie(cookies.Result().Cookies()[0])
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	var body struct {
		Compatibility string `json:"compatibility"`
		ClientUpdate  struct {
			Version string `json:"version"`
			Size    int    `json:"size"`
		} `json:"clientUpdate"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Compatibility != "client-update-required" || body.ClientUpdate.Version != "1.0.0" || body.ClientUpdate.Size != 7 {
		t.Fatalf("response=%+v", body)
	}

	unauthorized := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/updates/v1/client", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized client update code=%d", unauthorized.Code)
	}
	download := httptest.NewRequest(http.MethodGet, "/updates/v1/client", nil)
	download.AddCookie(cookies.Result().Cookies()[0])
	downloadResponse := httptest.NewRecorder()
	srv.Handler().ServeHTTP(downloadResponse, download)
	if downloadResponse.Code != http.StatusOK || downloadResponse.Body.String() != "package" {
		t.Fatalf("client update response code=%d body=%q", downloadResponse.Code, downloadResponse.Body.String())
	}
}

func TestNativeConfigOffersNewerEmbeddedClientWithCompatibleProtocol(t *testing.T) {
	a, err := auth.New(t.TempDir(), "test-password", 1)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(&config.Config{}, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetClientUpdate(&clientupdate.Bundle{
		Version: "1.0.0", Protocol: config.NativeVersion, SHA256: "abc", Data: []byte("package"),
	})
	request := httptest.NewRequest(http.MethodGet, "/native-config?av=0.9.0&nv="+config.NativeVersion, nil)
	cookies := httptest.NewRecorder()
	a.SetCookie(cookies)
	request.AddCookie(cookies.Result().Cookies()[0])
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	var body struct {
		Compatibility string `json:"compatibility"`
		ClientUpdate  struct {
			Version string `json:"version"`
		} `json:"clientUpdate"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Compatibility != "client-update-required" || body.ClientUpdate.Version != "1.0.0" {
		t.Fatalf("response=%+v", body)
	}
}

func TestNativeConfigDoesNotOfferCurrentEmbeddedClient(t *testing.T) {
	a, err := auth.New(t.TempDir(), "test-password", 1)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(&config.Config{}, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetClientUpdate(&clientupdate.Bundle{
		Version: "1.0.0", Protocol: config.NativeVersion, SHA256: "abc", Data: []byte("package"),
	})
	request := httptest.NewRequest(http.MethodGet, "/native-config?av=1.0.0&nv="+config.NativeVersion, nil)
	cookies := httptest.NewRecorder()
	a.SetCookie(cookies)
	request.AddCookie(cookies.Result().Cookies()[0])
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	var body struct {
		Compatibility string         `json:"compatibility"`
		ClientUpdate  map[string]any `json:"clientUpdate"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Compatibility != "compatible" || body.ClientUpdate != nil {
		t.Fatalf("response=%+v", body)
	}
}

func TestNativeConfigReturnsWSTicket(t *testing.T) {
	a, err := auth.New(t.TempDir(), "test-password", 1)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(&config.Config{ViewW: 1024, ViewH: 768}, a, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/native-config", nil)
	cookieResp := httptest.NewRecorder()
	a.SetCookie(cookieResp)
	req.AddCookie(cookieResp.Result().Cookies()[0])
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("native-config code = %d, want %d", resp.Code, http.StatusOK)
	}
	var body struct {
		Ticket string `json:"ticket"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(resp.Result().Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Token != "" {
		t.Fatal("native-config returned legacy token")
	}
	if !a.VerifyWSTicket(body.Ticket) {
		t.Fatal("native-config returned invalid ws ticket")
	}
}
