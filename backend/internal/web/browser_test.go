package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"surf-backend/internal/control"
	"surf-backend/internal/protocol"
)

type testBrowserControl struct{ calls int }

func (b *testBrowserControl) PrepareShutdown(revision uint64) error {
	if revision != 7 {
		return fmt.Errorf("state changed")
	}
	return nil
}

func (b *testBrowserControl) Status() protocol.BrowserModeEvent {
	return protocol.BrowserModeEvent{Type: "browser-mode", State: "setup", Revision: 7, Host: "Test"}
}
func (b *testBrowserControl) Request(action string, revision uint64, force bool) error {
	if revision != 7 {
		return fmt.Errorf("state changed")
	}
	b.calls++
	return nil
}

func TestBrowserControlsRequireAuthorityAndRevision(t *testing.T) {
	server, auth := newTestServer(t)
	browser := &testBrowserControl{}
	server.SetBrowserControl(browser)
	cookie := pairedCookie(t, auth)
	for _, test := range []struct {
		path, action  string
		revision      int
		paired, admin bool
		status        int
	}{
		{"/browser-mode", "resume", 7, false, false, 401},
		{"/admin/browser", "open", 7, true, false, 403},
		{"/browser-mode", "open", 7, true, false, 403},
		{"/browser-mode", "resume", 6, true, false, 409},
		{"/browser-mode", "resume", 7, true, false, 202},
		{"/admin/browser", "open", 7, false, true, 202},
	} {
		r := httptest.NewRequest(http.MethodPost, APIRoot+test.path, strings.NewReader(fmt.Sprintf(`{"action":%q,"revision":%d,"force":false}`, test.action, test.revision)))
		r.RemoteAddr = "127.0.0.1:12345"
		if test.paired {
			r.AddCookie(cookie)
		}
		if test.admin {
			r.Header.Set(control.AdminHeader, testAdminToken)
		}
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%+v: HTTP %d: %s", test, w.Code, w.Body.String())
		}
	}
	if browser.calls != 2 {
		t.Fatalf("unauthorized action reached browser: %d", browser.calls)
	}
}

func TestBrowserShutdownRejectsStaleConfirmation(t *testing.T) {
	for _, test := range []struct {
		revision string
		admin    bool
		status   int
	}{
		{"7", false, http.StatusForbidden},
		{"invalid", true, http.StatusConflict},
		{"6", true, http.StatusConflict},
		{"7", true, http.StatusAccepted},
	} {
		t.Run(fmt.Sprintf("%s/admin=%t", test.revision, test.admin), func(t *testing.T) {
			server, _ := newTestServer(t)
			server.SetBrowserControl(&testBrowserControl{})
			shutdown := make(chan struct{}, 1)
			server.SetShutdown(func() { shutdown <- struct{}{} })
			r := httptest.NewRequest(http.MethodPost, APIRoot+"/admin/shutdown", nil)
			r.RemoteAddr = "127.0.0.1:12345"
			r.Header.Set("X-Surf-Browser-Revision", test.revision)
			if test.admin {
				r.Header.Set(control.AdminHeader, testAdminToken)
			}
			w := httptest.NewRecorder()
			server.Handler().ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
			}
			if test.status == http.StatusAccepted {
				select {
				case <-shutdown:
				case <-time.After(time.Second):
					t.Fatal("confirmed shutdown was not called")
				}
			} else {
				select {
				case <-shutdown:
					t.Fatal("unconfirmed shutdown was called")
				default:
				}
			}
		})
	}
}
