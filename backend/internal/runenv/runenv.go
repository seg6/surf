// Package runenv owns host-mode process orchestration for surf-backend.
package runenv

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/proc"
)

type Runtime struct {
	children     []*exec.Cmd
	tempDirs     []string
	pulseModules []string
	pactlPath    string
	pactlEnv     []string
}

func Start(cfg *config.Config) (*Runtime, error) {
	r := &Runtime{}
	if cfg.RuntimeMode != "host" {
		cfg.RefreshChildEnv()
		return r, nil
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("host runtime is currently implemented only on Linux")
	}
	for _, dir := range []string{cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	_ = cleanupChromeSingletons(cfg.Profile)

	if cfg.ManageDisplay {
		display, err := pickDisplay()
		if err != nil {
			return nil, err
		}
		cfg.Display = display
		cfg.RefreshChildEnv()
		if err := r.startXvfb(cfg); err != nil {
			r.Shutdown()
			return nil, err
		}
	} else {
		cfg.RefreshChildEnv()
	}

	if cfg.ManagePulse {
		pulseDir, err := os.MkdirTemp("", "surf-pulse-*")
		if err != nil {
			r.Shutdown()
			return nil, err
		}
		r.tempDirs = append(r.tempDirs, pulseDir)
		cfg.PulseServer = "unix:" + filepath.Join(pulseDir, "native")
		cfg.RefreshChildEnv()
		if err := r.startPulse(cfg); err != nil {
			r.Shutdown()
			return nil, err
		}
	} else if cfg.EnsurePulseSink {
		if err := r.ensurePulseSink(cfg); err != nil {
			r.Shutdown()
			return nil, err
		}
	}
	return r, nil
}

func (r *Runtime) Shutdown() {
	for i := len(r.pulseModules) - 1; i >= 0; i-- {
		// Best effort: the user's Pulse/PipeWire server may already be gone.
		cmd := exec.Command(r.pactlPath, "unload-module", r.pulseModules[i])
		cmd.Env = append(os.Environ(), r.pactlEnv...)
		_ = cmd.Run()
	}
	for i := len(r.children) - 1; i >= 0; i-- {
		proc.Kill(r.children[i])
	}
	for _, dir := range r.tempDirs {
		_ = os.RemoveAll(dir)
	}
}

func cleanupChromeSingletons(profile string) error {
	entries, err := os.ReadDir(profile)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "Singleton") {
			_ = os.RemoveAll(filepath.Join(profile, e.Name()))
		}
	}
	return nil
}

func pickDisplay() (string, error) {
	start := 100 + int(time.Now().UnixNano()%500)
	for i := 0; i < 500; i++ {
		n := 100 + (start+i)%500
		if _, err := os.Stat(filepath.Join("/tmp/.X11-unix", "X"+strconv.Itoa(n))); os.IsNotExist(err) {
			return ":" + strconv.Itoa(n), nil
		}
	}
	return "", fmt.Errorf("no free X display found")
}

func displaySocket(display string) string {
	d := strings.TrimPrefix(display, ":")
	if i := strings.IndexByte(d, '.'); i >= 0 {
		d = d[:i]
	}
	return filepath.Join("/tmp/.X11-unix", "X"+d)
}

func (r *Runtime) startXvfb(cfg *config.Config) error {
	args := []string{
		cfg.Display,
		"-screen", "0", fmt.Sprintf("%dx%dx24", cfg.DisplayW, cfg.DisplayH),
		"-nolisten", "tcp",
	}
	cmd := proc.Command(cfg.XvfbPath, args...)
	cmd.Env = append(os.Environ(), cfg.ChildEnv...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Xvfb: %w", err)
	}
	r.children = append(r.children, cmd)
	go logExit("Xvfb", cmd)

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(displaySocket(cfg.Display)); err == nil {
			log.Printf("runtime: Xvfb started on %s", cfg.Display)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("Xvfb did not create %s", displaySocket(cfg.Display))
}

func (r *Runtime) startPulse(cfg *config.Config) error {
	socket := strings.TrimPrefix(cfg.PulseServer, "unix:")
	args := []string{
		"--daemonize=no",
		"--exit-idle-time=-1",
		"--disallow-exit=true",
		"--load=module-native-protocol-unix socket=" + socket + " auth-anonymous=1",
		"--load=module-null-sink sink_name=" + cfg.PulseSink + " sink_properties=device.description=Surf",
	}
	cmd := proc.Command(cfg.PulseaudioPath, args...)
	cmd.Env = append(os.Environ(), cfg.ChildEnv...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start pulseaudio: %w", err)
	}
	r.children = append(r.children, cmd)
	go logExit("pulseaudio", cmd)

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			log.Printf("runtime: PulseAudio started at %s", cfg.PulseServer)
			r.runPactl(cfg, "set-default-sink", cfg.PulseSink)
			r.runPactl(cfg, "set-sink-volume", cfg.PulseSink, "100%")
			r.runPactl(cfg, "set-sink-mute", cfg.PulseSink, "0")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("pulseaudio did not create %s", socket)
}

func (r *Runtime) ensurePulseSink(cfg *config.Config) error {
	if cfg.PulseSink == "" {
		return nil
	}
	r.pactlPath = cfg.PactlPath
	r.pactlEnv = append([]string(nil), cfg.ChildEnv...)
	if r.pulseSinkExists(cfg) {
		log.Printf("runtime: PulseAudio sink %s already exists", cfg.PulseSink)
		return nil
	}
	args := []string{"load-module", "module-null-sink", "sink_name=" + cfg.PulseSink, "sink_properties=device.description=Surf"}
	cmd := exec.Command(cfg.PactlPath, args...)
	cmd.Env = append(os.Environ(), cfg.ChildEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pactl %s failed: %w %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	moduleID := strings.TrimSpace(string(out))
	if moduleID != "" {
		r.pulseModules = append(r.pulseModules, moduleID)
	}
	log.Printf("runtime: created PulseAudio null sink %s", cfg.PulseSink)
	return nil
}

func (r *Runtime) pulseSinkExists(cfg *config.Config) bool {
	cmd := exec.Command(cfg.PactlPath, "list", "short", "sinks")
	cmd.Env = append(os.Environ(), cfg.ChildEnv...)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == cfg.PulseSink {
			return true
		}
	}
	return false
}

func (r *Runtime) runPactl(cfg *config.Config, args ...string) {
	cmd := exec.Command(cfg.PactlPath, args...)
	cmd.Env = append(os.Environ(), cfg.ChildEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("runtime: pactl %s failed: %v %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

func logExit(name string, cmd *exec.Cmd) {
	err := cmd.Wait()
	if err != nil {
		log.Printf("runtime: %s exited: %v", name, err)
	}
}
