//go:build linux

// Package runenv's Linux platform drives everything surf-backend needs on a
// bare Linux host: a private Xvfb display for Chromium, x11grab/xrandr for
// the H.264 lane, and a PulseAudio-compatible sink for the PCM lane.
package runenv

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	if cfg.ManageDisplay {
		checks = append(checks, checkTool("Xvfb", cfg.XvfbPath, true))
	}
	checks = append(checks, checkTool("xrandr", cfg.XrandrPath, true))
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

// linuxHandle is the live Linux runtime: whatever Xvfb/PulseAudio processes
// Prepare started, plus the config needed to build xrandr/ffmpeg commands
// later.
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
	if _, err := exec.LookPath(cfg.XrandrPath); err != nil {
		return fmt.Errorf("xrandr is required: %w", err)
	}
	for _, dir := range []string{cfg.Profile, cfg.DownloadsDir, cfg.UploadsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	_ = cleanupChromeSingletons(cfg.Profile)

	if cfg.ManageDisplay {
		display, err := pickDisplay()
		if err != nil {
			return err
		}
		cfg.Display = display
		cfg.RefreshChildEnv()
		if err := lh.startXvfb(cfg); err != nil {
			return err
		}
	} else {
		cfg.RefreshChildEnv()
	}

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

// ChromeArgs forces the X11 ozone backend: without it Chromium treats the
// Xvfb window as backgrounded/occluded and stops producing compositor
// frames (dead screencast, multi-second screenshots).
func (lh *linuxHandle) ChromeArgs() []string {
	return []string{"--ozone-platform=x11"}
}

// VideoCaptureArgs grabs the X display directly with x11grab, bypassing CDP
// entirely for the low-latency H.264 lane.
func (lh *linuxHandle) VideoCaptureArgs(surface string, w, h, fps int) []string {
	return []string{
		"-loglevel", "warning",
		// No input buffering/probing: x11grab is raw frames, every ms of
		// demuxer buffer is glass-to-glass latency.
		"-fflags", "nobuffer",
		"-probesize", "32",
		"-f", "x11grab",
		"-draw_mouse", "0", // the X cursor is CDP's phantom, not the user's
		"-framerate", fmt.Sprint(fps),
		"-video_size", fmt.Sprintf("%dx%d", w, h),
		"-i", surface,
	}
}

// AudioCaptureArgs grabs the PulseAudio-compatible monitor source Chromium's
// output was routed to.
func (lh *linuxHandle) AudioCaptureArgs(source string) []string {
	return []string{"-loglevel", "warning", "-f", "pulse", "-i", source}
}

// ResizeSurface best-effort resizes the live X screen (RANDR) to the client
// viewport. Xvfb also starts on an oversized canvas, so the video lane can
// still grab the correctly-sized top-left viewport even when this X server
// refuses custom RANDR modes. Blocking (three tiny X round-trips); callers
// keep this off hot paths.
func (lh *linuxHandle) ResizeSurface(w, h int) error {
	xrandrPath := lh.cfg.XrandrPath
	if xrandrPath == "" {
		xrandrPath = "xrandr"
	}
	output := lh.cfg.XOutput
	if output == "" {
		output = "screen"
	}
	env := lh.cfg.ChildEnv
	name := fmt.Sprintf("%dx%d", w, h)
	ht, vt := w+64, h+16
	clock := float64(ht) * float64(vt) * 60.0 / 1e6 // ~60Hz; Xvfb doesn't care
	// Create + attach the mode; both fail harmlessly when it already exists.
	cmd := exec.Command(xrandrPath, "--newmode", name, fmt.Sprintf("%.2f", clock),
		fmt.Sprint(w), fmt.Sprint(w+16), fmt.Sprint(w+32), fmt.Sprint(ht),
		fmt.Sprint(h), fmt.Sprint(h+3), fmt.Sprint(h+6), fmt.Sprint(vt))
	cmd.Env = append(os.Environ(), env...)
	_ = cmd.Run()
	cmd = exec.Command(xrandrPath, "--addmode", output, name)
	cmd.Env = append(os.Environ(), env...)
	_ = cmd.Run()
	cmd = exec.Command(xrandrPath, "--output", output, "--mode", name)
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Run(); err != nil {
		return err
	}
	log.Printf("screen: X display resized to %s", name)
	return nil
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

func (lh *linuxHandle) startXvfb(cfg *config.Config) error {
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
	lh.children = append(lh.children, cmd)
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

// HiddenDesktop is a Windows-only concept: no-op here, Chromium always
// launches on the (already invisible, since it's inside Xvfb) X display.
func (lh *linuxHandle) HiddenDesktop() string { return "" }
