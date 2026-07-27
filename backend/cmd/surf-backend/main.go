package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/grandcat/zeroconf"

	"surf-backend/internal/auth"
	"surf-backend/internal/browser"
	"surf-backend/internal/browserbin"
	"surf-backend/internal/config"
	"surf-backend/internal/diagnostics"
	"surf-backend/internal/ffmpegbin"
	"surf-backend/internal/httpd"
	"surf-backend/internal/protocol"
	"surf-backend/internal/runenv"
	"surf-backend/internal/ws"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("missing command")
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "serve":
		if len(args) != 0 {
			return fmt.Errorf("serve takes no arguments")
		}
		return serve()
	case "doctor":
		if len(args) != 0 {
			return fmt.Errorf("doctor takes no arguments")
		}
		return doctor()
	case "version":
		if len(args) != 0 {
			return fmt.Errorf("version takes no arguments")
		}
		fmt.Printf("surf-backend %s\nprotocol %s\n", config.AppVersion, config.NativeVersion)
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  surf-backend serve
  surf-backend doctor
  surf-backend version
`)
}

func doctor() error {
	cfg, err := config.LoadForDiagnostics()
	if err != nil {
		return err
	}
	if err := ensureBrowser(cfg); err != nil {
		return err
	}
	if err := ensureFFmpeg(cfg); err != nil {
		return err
	}
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

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := ensureBrowser(cfg); err != nil {
		return err
	}
	if err := ensureFFmpeg(cfg); err != nil {
		return err
	}

	rt, err := runenv.Start(cfg)
	if err != nil {
		return err
	}
	defer rt.Shutdown()

	a, err := auth.New(cfg.Profile, cfg.SurfPassword, cfg.AuthDays)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	hub := ws.NewHub()
	b := browser.New(cfg, hub, rt.Handle())
	hub.SetHandler(b)
	defer b.Shutdown()

	if err := b.Start(); err != nil {
		return err
	}
	if os.Getenv("SURF_ADVERTISE") != "0" {
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
	diagnosticsManager, err := diagnostics.New(cfg.SurfHome, b.Stats)
	if err != nil {
		return fmt.Errorf("diagnostics: %w", err)
	}
	diagnosticsManager.SetCaptureNotifier(func(active bool) {
		hub.BroadcastJSON(protocol.CaptureStateEvent{Type: "capture-state", Active: active})
	})
	diagnosticsManager.Register(srv)
	log.Printf("surf-backend listening on %s:%d", cfg.BindAddr, cfg.Port)

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

func ensureBrowser(cfg *config.Config) error {
	if cfg.ChromePath != "" {
		log.Printf("runtime: using CHROME=%s", cfg.ChromePath)
		return nil
	}
	log.Printf("runtime: ensuring Chrome for Testing %s (--headless=new)", browserbin.Version)
	path, err := browserbin.EnsureChrome(cfg.SurfHome)
	if err != nil {
		return err
	}
	cfg.ChromePath = path
	log.Printf("runtime: using managed Chrome %s", path)
	return nil
}

func ensureFFmpeg(cfg *config.Config) error {
	if cfg.FFmpegPath != "" {
		log.Printf("runtime: using FFMPEG=%s", cfg.FFmpegPath)
		return nil
	}
	log.Printf("runtime: ensuring FFmpeg %s", ffmpegbin.Version)
	path, err := ffmpegbin.Ensure(cfg.SurfHome)
	if err != nil {
		return err
	}
	cfg.FFmpegPath = path
	log.Printf("runtime: using managed FFmpeg %s", path)
	return nil
}
