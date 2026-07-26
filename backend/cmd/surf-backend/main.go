package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/grandcat/zeroconf"

	"surf-backend/internal/auth"
	"surf-backend/internal/browser"
	"surf-backend/internal/config"
	"surf-backend/internal/httpd"
	"surf-backend/internal/ws"
)

func main() {
	hashPassword := flag.String("hash-password", "", "print a bcrypt hash for a Surf password and exit")
	flag.Parse()
	if *hashPassword != "" {
		hash, err := auth.HashPassword(*hashPassword)
		if err != nil {
			log.Fatalf("hash password: %v", err)
		}
		fmt.Println(hash)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("fatal: %v", err)
	}
	a := auth.New(cfg.Profile, cfg.AuthHash, cfg.AuthDays)
	hub := ws.NewHub()
	b := browser.New(cfg, hub)
	hub.SetHandler(b)

	if err := b.Start(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
	if os.Getenv("SURF_ADVERTISE") == "1" {
		advertisePort := cfg.Port
		if v, err := strconv.Atoi(os.Getenv("SURF_ADVERTISE_PORT")); err == nil && v > 0 {
			advertisePort = v
		}
		ad, err := zeroconf.Register("Surf", "_surf._tcp", "local.", advertisePort,
			[]string{"path=/", "proto=http", "app=surf-backend", "nv=" + config.NativeVersion}, nil)
		if err != nil {
			log.Printf("bonjour advertise failed: %v", err)
		} else {
			defer ad.Shutdown()
			log.Printf("bonjour advertised Surf on _surf._tcp port %d", advertisePort)
		}
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
	srv.SetHealthCheck(b.Health)
	srv.SetStats(b.Stats)
	b.RegisterRoutes(srv)
	log.Printf("surf-backend listening on %d", cfg.Port)
	log.Fatal(httpd.Listen(cfg.Port, srv.Handler()))
}
