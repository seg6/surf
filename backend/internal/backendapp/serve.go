// Package backendapp runs the Surf backend independently of its command-line
// entrypoint, allowing the desktop supervisor to carry the server in one file.
package backendapp

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/grandcat/zeroconf"

	"surf-backend/internal/auth"
	"surf-backend/internal/browser"
	"surf-backend/internal/browserbin"
	"surf-backend/internal/clientupdate"
	"surf-backend/internal/config"
	"surf-backend/internal/contentblocker"
	"surf-backend/internal/ffmpegbin"
	"surf-backend/internal/httpd"
	"surf-backend/internal/runenv"
	"surf-backend/internal/ws"
)

func Serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := EnsureRuntime(cfg); err != nil {
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
	b, err := browser.New(cfg, hub, rt.Handle())
	if err != nil {
		return err
	}
	hub.SetHandler(b)
	defer b.Shutdown()
	if err := b.Start(); err != nil {
		return err
	}
	if os.Getenv("SURF_ADVERTISE") != "0" {
		port := cfg.Port
		if value, err := strconv.Atoi(os.Getenv("SURF_ADVERTISE_PORT")); err == nil && value > 0 {
			port = value
		}
		ad, err := zeroconf.Register("Surf", "_surf._tcp", "local.", port,
			[]string{"path=/", "proto=http", "app=surf", "nv=" + config.NativeVersion}, nil)
		if err != nil {
			log.Printf("bonjour advertise failed: %v", err)
		} else {
			defer ad.Shutdown()
			log.Printf("bonjour advertised Surf on _surf._tcp port %d", port)
		}
	}
	srv, err := httpd.New(cfg, a, hub)
	if err != nil {
		return err
	}
	srv.SetHealthCheck(b.Health)
	srv.SetStats(b.Stats)
	if bundle := clientupdate.Current(); bundle != nil {
		srv.SetClientUpdate(bundle)
		log.Printf("updates: embedded iPad client %s protocol %s (%d bytes)", bundle.Version, bundle.Protocol, len(bundle.Data))
	}
	b.RegisterRoutes(srv)
	log.Printf("surf listening on %s:%d", cfg.BindAddr, cfg.Port)

	serverErr := make(chan error, 1)
	go func() { serverErr <- httpd.Listen(cfg.BindAddr, cfg.Port, srv.Handler()) }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		return err
	case <-b.Died():
		return fmt.Errorf("chromium connection lost")
	case signal := <-sig:
		log.Printf("received %s, shutting down", signal)
		return nil
	}
}

func EnsureRuntime(cfg *config.Config) error {
	if err := ensureBrowser(cfg); err != nil {
		return err
	}
	return ensureFFmpeg(cfg)
}

func ensureBrowser(cfg *config.Config) error {
	if cfg.ChromePath != "" {
		log.Printf("runtime: using CHROME=%s", cfg.ChromePath)
	} else {
		log.Printf("runtime: resolving Chrome/Chromium (--headless=new)")
		var path, source string
		var err error
		path, source, err = browserbin.ResolveExtensionCapable(cfg.SurfHome)
		if err != nil {
			return err
		}
		cfg.ChromePath = path
		log.Printf("runtime: using %s at %s", source, path)
		if strings.HasPrefix(source, "managed ") {
			go func() {
				if err := browserbin.UpdateManaged(cfg.SurfHome); err != nil {
					log.Printf("runtime: managed browser update check failed: %v", err)
				}
			}()
		}
	}
	if cfg.ContentBlocker {
		log.Printf("runtime: ensuring uBlock Origin Lite %s", contentblocker.Version)
		path, err := contentblocker.Ensure(cfg.SurfHome)
		if err != nil {
			return err
		}
		cfg.ContentBlockerPath = path
		log.Printf("runtime: content blocker ready at %s", path)
	}
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
