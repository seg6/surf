package main

import (
	"log"

	"rbrowser/internal/auth"
	"rbrowser/internal/browser"
	"rbrowser/internal/config"
	"rbrowser/internal/httpd"
	"rbrowser/internal/ws"
)

func main() {
	cfg := config.Load()
	a := auth.New(cfg.Profile, cfg.AuthHash, cfg.AuthDays)
	hub := ws.NewHub()
	b := browser.New(cfg, hub)
	hub.SetHandler(b)

	if err := b.Start(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
	go func() {
		<-b.Died()
		// Chromium (or its DevTools socket) is gone; die and let Docker restart us.
		log.Fatal("chromium connection lost")
	}()

	srv, err := httpd.New(cfg, a, hub)
	if err != nil {
		log.Fatalf("fatal: %v", err)
	}
	b.RegisterRoutes(srv)
	log.Printf("rbrowser listening on %d", cfg.Port)
	log.Fatal(httpd.Listen(cfg.Port, srv.Handler()))
}
