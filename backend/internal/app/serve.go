// Package app assembles and runs the Surf backend independently of its command-line
// entrypoint, allowing the desktop supervisor to carry the server in one file.
package app

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"surf-backend/internal/auth"
	"surf-backend/internal/browser"
	"surf-backend/internal/chromium"
	"surf-backend/internal/config"
	"surf-backend/internal/contentblocker"
	"surf-backend/internal/control"
	"surf-backend/internal/discovery"
	"surf-backend/internal/identity"
	"surf-backend/internal/process"
	"surf-backend/internal/transport"
	"surf-backend/internal/web"
)

func Serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	instance, acquired, err := process.AcquireInstanceLock(
		filepath.Join(cfg.SurfHome, "server.lock"))
	if err != nil {
		return fmt.Errorf("lock Surf backend: %w", err)
	}
	if !acquired {
		return fmt.Errorf("another Surf backend is already using %s", cfg.Profile)
	}
	defer instance.Close()
	if err := Prepare(cfg); err != nil {
		return err
	}

	releaseChildren := process.ProtectChildren()
	defer releaseChildren()
	ident, err := identity.LoadOrCreate(cfg.SurfHome, cfg.ServerName)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	a, err := auth.New(cfg.SurfHome, ident.Fingerprint)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	hub := transport.New()
	a.SetRevokeHandler(hub.CloseDevice)
	b, err := browser.New(cfg, hub)
	if err != nil {
		return err
	}
	hub.SetHandler(b)
	defer b.Shutdown()
	if err := b.Start(); err != nil {
		return err
	}
	srv := web.New(cfg, a, ident, hub)
	srv.SetHealthCheck(b.Health)
	srv.SetStats(b.Stats)
	b.RegisterRoutes(srv)
	publicListener, err := net.Listen("tcp", net.JoinHostPort(cfg.BindAddr, fmt.Sprint(cfg.Port)))
	if err != nil {
		return fmt.Errorf("listen on %s:%d: %w", cfg.BindAddr, cfg.Port, err)
	}
	defer publicListener.Close()
	controlListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for local control: %w", err)
	}
	defer controlListener.Close()
	descriptor, err := control.New("https://"+controlListener.Addr().String(), ident.Fingerprint, config.NativeVersion, cfg.Port)
	if err != nil {
		return err
	}
	srv.SetAdminToken(descriptor.AdminToken)

	serverErr := make(chan error, 2)
	go func() {
		serverErr <- fmt.Errorf("public listener: %w", web.ServeTLS(publicListener, ident, srv.Handler()))
	}()
	go func() {
		serverErr <- fmt.Errorf("control listener: %w", web.ServeTLS(controlListener, ident, srv.Handler()))
	}()
	if err := control.Write(cfg.SurfHome, descriptor); err != nil {
		return err
	}
	defer func() {
		if err := control.RemoveOwned(cfg.SurfHome, descriptor.AdminToken); err != nil {
			log.Printf("remove daemon descriptor: %v", err)
		}
	}()
	if os.Getenv("SURF_ADVERTISE") != "0" {
		port := cfg.Port
		if value, err := strconv.Atoi(os.Getenv("SURF_ADVERTISE_PORT")); err == nil && value > 0 {
			port = value
		}
		ad, err := discovery.Register(cfg.ServerName, port,
			[]string{"path=" + web.APIRoot, "proto=https", "api=v1", "id=" + ident.Fingerprint, "name=" + cfg.ServerName, "nv=" + config.NativeVersion})
		if err != nil {
			log.Printf("bonjour advertise failed: %v", err)
		} else {
			defer ad.Shutdown()
			log.Printf("bonjour advertised Surf on _surf._tcp port %d", port)
		}
	}
	log.Printf("surf daemon listening with TLS on %s:%d identity=%s", cfg.BindAddr, cfg.Port, ident.Fingerprint)
	log.Printf("surf daemon control endpoint %s", descriptor.ControlURL)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
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

// Prepare resolves the browser and optional managed extensions.
func Prepare(cfg *config.Config) error {
	for _, dir := range []string{cfg.SurfHome, cfg.DownloadsDir, cfg.UploadsDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", dir, err)
		}
	}
	if err := chromium.PrepareProfile(cfg.Profile); err != nil {
		return err
	}
	if cfg.ChromePath != "" {
		log.Printf("runtime: using CHROME=%s", cfg.ChromePath)
	} else {
		log.Printf("runtime: resolving Chrome/Chromium (--headless=new)")
		var path, source string
		var err error
		path, source, err = chromium.Resolve(cfg.SurfHome)
		if err != nil {
			return err
		}
		cfg.ChromePath = path
		log.Printf("runtime: using %s at %s", source, path)
		if strings.HasPrefix(source, "managed ") {
			go func() {
				if err := chromium.UpdateManaged(cfg.SurfHome); err != nil {
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
