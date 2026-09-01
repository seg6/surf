package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestActivateDesktopInstance(t *testing.T) {
	activated := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/activate" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		activated <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	home := t.TempDir()
	if err := writeDesktopInstance(home, server.URL); err != nil {
		t.Fatal(err)
	}
	if err := activateDesktopInstance(home); err != nil {
		t.Fatal(err)
	}
	select {
	case <-activated:
	default:
		t.Fatal("running instance was not activated")
	}
}

func TestQuitDesktopInstance(t *testing.T) {
	quit := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/quit" || r.Method != http.MethodPost {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Surf-Desktop") != "1" {
			t.Errorf("desktop header=%q", r.Header.Get("X-Surf-Desktop"))
		}
		quit <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	home := t.TempDir()
	if err := writeDesktopInstance(home, server.URL); err != nil {
		t.Fatal(err)
	}
	if err := quitDesktopInstance(home); err != nil {
		t.Fatal(err)
	}
	select {
	case <-quit:
	default:
		t.Fatal("running instance was not asked to quit")
	}
}
