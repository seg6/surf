package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/grandcat/zeroconf"

	"surf-backend/internal/auth"
	"surf-backend/internal/browser"
	"surf-backend/internal/config"
	"surf-backend/internal/httpd"
	"surf-backend/internal/runenv"
	"surf-backend/internal/ws"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	hashPassword := flag.String("hash-password", "", "print a bcrypt hash for a Surf password and exit")
	doctor := flag.Bool("doctor", false, "check configured host-mode runtime tools and exit")
	flag.Parse()
	if *hashPassword != "" {
		hash, err := auth.HashPassword(*hashPassword)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		fmt.Println(hash)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *doctor {
		failed := false
		for _, check := range runenv.Doctor(cfg) {
			if check.OK {
				log.Printf("doctor: ok %s=%s", check.Name, check.Path)
				continue
			}
			if check.Required {
				failed = true
				log.Printf("doctor: missing %s=%s: %v", check.Name, check.Path, check.Err)
				continue
			}
			log.Printf("doctor: optional missing %s=%s: %v", check.Name, check.Path, check.Err)
		}
		if failed {
			return fmt.Errorf("doctor failed")
		}
		return nil
	}

	rt, err := runenv.Start(cfg)
	if err != nil {
		return err
	}
	defer rt.Shutdown()

	a := auth.New(cfg.Profile, cfg.AuthHash, cfg.AuthDays)
	hub := ws.NewHub()
	b := browser.New(cfg, hub)
	hub.SetHandler(b)
	defer b.Shutdown()

	if err := b.Start(); err != nil {
		return err
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
	srv, err := httpd.New(cfg, a, hub)
	if err != nil {
		return err
	}
	srv.SetHealthCheck(b.Health)
	srv.SetStats(b.Stats)
	b.RegisterRoutes(srv)
	log.Printf("surf-backend listening on %s:%d runtime=%s", cfg.BindAddr, cfg.Port, cfg.RuntimeMode)

	serverErr := make(chan error, 1)
	go func() { serverErr <- httpd.Listen(cfg.BindAddr, cfg.Port, srv.Handler()) }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		return err
	case <-b.Died():
		return fmt.Errorf("chromium connection lost")
	case s := <-sig:
		log.Printf("received %s, shutting down", s)
		return nil
	}
}
