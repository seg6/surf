//go:build linux

// Package runenv's Linux platform brings up a PulseAudio-compatible sink for
// the PCM lane. Chromium runs headless (no display server needed at all —
// see internal/cdp), and the H.264 lane transcodes CDP's own screencast
// frames instead of grabbing the X display, so this is the only host
// service left to manage here.
package runenv

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"surf-backend/internal/config"
	"surf-backend/internal/proc"
)

func newPlatform() Platform { return linuxPlatform{} }

type linuxPlatform struct{}

func (linuxPlatform) Doctor(cfg *config.Config) []Check {
	checks := []Check{
		checkTool("chromium", cfg.ChromePath, true),
		checkTool("ffmpeg", cfg.FFmpegPath, true),
	}
	if cfg.ManagePulse {
		checks = append(checks, checkTool("pulseaudio", cfg.PulseaudioPath, true), checkTool("pactl", cfg.PactlPath, true))
	} else if cfg.EnsurePulseSink {
		checks = append(checks, checkTool("pactl", cfg.PactlPath, true))
	}
	return checks
}

func (linuxPlatform) Prepare(cfg *config.Config) (Handle, error) {
	lh := &linuxHandle{cfg: cfg}
	if err := lh.prepare(); err != nil {
		lh.Shutdown()
		return nil, err
	}
	return lh, nil
}

// linuxHandle is the live Linux runtime: whatever PulseAudio process
// Prepare started, plus the config needed to build ffmpeg's audio-capture
// command later.
type linuxHandle struct {
	cfg *config.Config

	children     []*exec.Cmd
	tempDirs     []string
	pulseModules []string
	pactlPath    string
	pactlEnv     []string
}

func (lh *linuxHandle) prepare() error {
	cfg := lh.cfg
	for _, dir := range []string{cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	_ = cleanupChromeSingletons(cfg.Profile)
	cfg.RefreshChildEnv()

	if cfg.ManagePulse {
		pulseDir, err := os.MkdirTemp("", "surf-pulse-*")
		if err != nil {
			return err
		}
		lh.tempDirs = append(lh.tempDirs, pulseDir)
		cfg.PulseServer = "unix:" + filepath.Join(pulseDir, "native")
		cfg.RefreshChildEnv()
		if err := lh.startPulse(cfg); err != nil {
			return err
		}
	} else if cfg.EnsurePulseSink {
		if err := lh.ensurePulseSink(cfg); err != nil {
			return err
		}
	}
	return nil
}

func (lh *linuxHandle) Shutdown() {
	for i := len(lh.pulseModules) - 1; i >= 0; i-- {
		// Best effort: the user's Pulse/PipeWire server may already be gone.
		cmd := exec.Command(lh.pactlPath, "unload-module", lh.pulseModules[i])
		cmd.Env = append(os.Environ(), lh.pactlEnv...)
		_ = cmd.Run()
	}
	for i := len(lh.children) - 1; i >= 0; i-- {
		proc.Kill(lh.children[i].Process.Pid)
	}
	for _, dir := range lh.tempDirs {
		_ = os.RemoveAll(dir)
	}
}

// AudioCaptureArgs grabs the PulseAudio-compatible monitor source Chromium's
// output was routed to.
func (lh *linuxHandle) AudioCaptureArgs(source string) []string {
	return []string{"-loglevel", "warning", "-f", "pulse", "-i", source}
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

func (lh *linuxHandle) startPulse(cfg *config.Config) error {
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
	lh.children = append(lh.children, cmd)
	go logExit("pulseaudio", cmd)

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			log.Printf("runtime: PulseAudio started at %s", cfg.PulseServer)
			lh.runPactl(cfg, "set-default-sink", cfg.PulseSink)
			lh.runPactl(cfg, "set-sink-volume", cfg.PulseSink, "100%")
			lh.runPactl(cfg, "set-sink-mute", cfg.PulseSink, "0")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("pulseaudio did not create %s", socket)
}

func (lh *linuxHandle) ensurePulseSink(cfg *config.Config) error {
	if cfg.PulseSink == "" {
		return nil
	}
	lh.pactlPath = cfg.PactlPath
	lh.pactlEnv = append([]string(nil), cfg.ChildEnv...)
	if lh.pulseSinkExists(cfg) {
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
		lh.pulseModules = append(lh.pulseModules, moduleID)
	}
	log.Printf("runtime: created PulseAudio null sink %s", cfg.PulseSink)
	return nil
}

func (lh *linuxHandle) pulseSinkExists(cfg *config.Config) bool {
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

func (lh *linuxHandle) runPactl(cfg *config.Config, args ...string) {
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
