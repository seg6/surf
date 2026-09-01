package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"
	"unicode"

	"surf-backend/internal/auth"
	"surf-backend/internal/clipboard"
	"surf-backend/internal/config"
	"surf-backend/internal/control"
	"surf-backend/internal/logstore"
	"surf-backend/internal/web"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/term"
)

type localAdmin struct {
	client     *http.Client
	base       string
	home       string
	descriptor control.Descriptor
}

func newLocalAdmin() (*localAdmin, error) {
	home, err := surfHome()
	if err != nil {
		return nil, err
	}
	descriptor, err := control.Load(home)
	if err != nil {
		return nil, fmt.Errorf("Surf server is not running for SURF_HOME=%s: %w", home, err)
	}
	want := descriptor.ServerID
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // VerifyConnection pins Surf's local identity below.
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return fmt.Errorf("backend presented no certificate")
			}
			sum := sha256.Sum256(state.PeerCertificates[0].Raw)
			if got := hex.EncodeToString(sum[:]); got != want {
				return fmt.Errorf("backend identity mismatch: got %s", got)
			}
			return nil
		},
	}
	return &localAdmin{
		client: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig}},
		base:   descriptor.ControlURL, home: home, descriptor: descriptor,
	}, nil
}

func (a *localAdmin) request(method, path string, body any, target any) error {
	data, err := a.requestBytes(method, path, body)
	if err != nil {
		return err
	}
	if target != nil {
		if err := json.Unmarshal(data, target); err != nil {
			return err
		}
	}
	return nil
}

func (a *localAdmin) requestBytes(method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, a.base+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set(control.AdminHeader, a.descriptor.AdminToken)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Surf server is not running at %s: %w", a.base, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("backend returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func runStatusCommand() error {
	admin, err := newLocalAdmin()
	if err != nil {
		return err
	}
	if err := admin.request(http.MethodGet, web.APIRoot+"/health", nil, nil); err != nil {
		return err
	}
	var server struct {
		Name                string `json:"name"`
		ServerID            string `json:"serverID"`
		Compatibility       int    `json:"compatibilityVersion"`
		LegacyCompatibility string `json:"protocol"`
		Version             string `json:"version"`
	}
	if err := admin.request(http.MethodGet, web.APIRoot+"/server", nil, &server); err != nil {
		return err
	}
	var devices struct {
		Devices []auth.Device `json:"devices"`
	}
	if err := admin.request(http.MethodGet, web.APIRoot+"/admin/devices", nil, &devices); err != nil {
		return err
	}
	fmt.Println("Surf server is running.")
	fmt.Printf("Name: %s\n", terminalText(server.Name))
	fmt.Printf("PID: %d\n", admin.descriptor.PID)
	fmt.Printf("Port: %d\n", admin.descriptor.PublicPort)
	compatibility := server.LegacyCompatibility
	if server.Compatibility > 0 {
		compatibility = fmt.Sprint(server.Compatibility)
	} else if generation, ok := config.ParseClientCompatibility("", server.LegacyCompatibility); ok {
		compatibility = fmt.Sprint(generation)
	}
	if compatibility == "" {
		compatibility = "unknown"
	}
	fmt.Printf("Version: %s\nCompatibility: %s\n", server.Version, compatibility)
	fmt.Printf("Server ID: %s\n", server.ServerID)
	fmt.Printf("Paired devices: %d\n", len(devices.Devices))
	return nil
}

func runQuitCommand() error {
	home, err := surfHome()
	if err != nil {
		return err
	}
	lock, primary, err := acquireDesktopInstance(home)
	if err != nil {
		return err
	}
	if !primary {
		if err := quitDesktopInstance(home); err != nil {
			return err
		}
		fmt.Println("Surf is closing.")
		return nil
	}
	if err := lock.Close(); err != nil {
		return err
	}
	admin, err := newLocalAdmin()
	if errors.Is(err, control.ErrNotRunning) {
		fmt.Println("Surf is not running.")
		return nil
	}
	if err != nil {
		return err
	}
	if err := admin.request(http.MethodPost, web.APIRoot+"/admin/shutdown", nil, nil); err != nil {
		return err
	}
	fmt.Println("Surf is closing.")
	return nil
}

func runPairCommand() error {
	ctx, cancel := signalContext()
	defer cancel()
	return runPairCommandContext(ctx)
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func runPairCommandContext(ctx context.Context) error {
	admin, err := newLocalAdmin()
	if err != nil {
		return err
	}
	address := pairCommandAddress(admin.descriptor.PublicPort)
	var session struct {
		Active     bool   `json:"active"`
		ServerID   string `json:"serverID"`
		Name       string `json:"name"`
		Address    string `json:"publicAddress"`
		PairingURL string `json:"pairingURL"`
		Code       string `json:"code"`
	}
	if err := admin.request(http.MethodPost, web.APIRoot+"/admin/pairing/session", map[string]string{"publicAddress": address}, &session); err != nil {
		return err
	}
	defer func() { _ = admin.request(http.MethodDelete, web.APIRoot+"/admin/pairing/session", nil, nil) }()
	address = strings.TrimSpace(session.Address)
	fmt.Printf("Server: %s\nFingerprint: %s\n", terminalText(session.Name), session.ServerID)
	fmt.Printf("Pairing code: %s\n", session.Code)
	fmt.Println("Need the iOS app? Open https://seg6.space/surf/ on the device.")
	fmt.Println("On the client, scan this QR code. On older devices, enter the server address and six-digit code.")
	if address != "" {
		fmt.Printf("Manual address: %s\n", address)
		if code, err := qrcode.New(session.PairingURL, qrcode.Medium); err == nil {
			fmt.Print(code.ToSmallString(false))
		}
	}
	fmt.Println("Only this one-time code can pair one device. Press Ctrl+C to cancel it.")

	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	shown := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nPairing cancelled.")
			return nil
		case <-ticker.C:
		}
		var response struct {
			Candidates []auth.PairingStatus `json:"candidates"`
		}
		if err := admin.request(http.MethodGet, web.APIRoot+"/admin/pairing/candidates", nil, &response); err != nil {
			return err
		}
		for _, candidate := range response.Candidates {
			if candidate.Paired {
				fmt.Printf("Device %s paired.\n", terminalText(candidate.DeviceName))
				return nil
			}
			if shown[candidate.ID] {
				continue
			}
			shown[candidate.ID] = true
			deviceName := terminalText(candidate.DeviceName)
			fmt.Printf("\nDevice: %s\nVerification phrase: %s\n", deviceName, terminalText(candidate.Phrase))
			fmt.Println("Compare these words on the client, then confirm there. The code is already consumed.")
		}
	}
}

func pairCommandAddress(port int) string {
	if address := strings.TrimSpace(os.Getenv("SURF_PUBLIC_ADDRESS")); address != "" {
		return address
	}
	if urls := localLANURLs(port); len(urls) != 0 {
		return strings.TrimPrefix(urls[0], "https://")
	}
	return ""
}

func runDevicesCommand(args []string) error {
	admin, err := newLocalAdmin()
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "list" {
		var response struct {
			Devices []auth.Device `json:"devices"`
		}
		if err := admin.request(http.MethodGet, web.APIRoot+"/admin/devices", nil, &response); err != nil {
			return err
		}
		if len(response.Devices) == 0 {
			fmt.Println("No paired devices.")
			return nil
		}
		for _, device := range response.Devices {
			last := "never"
			if !device.LastSeen.IsZero() {
				last = device.LastSeen.Local().Format(time.RFC3339)
			}
			fmt.Printf("%s\t%s\tpaired %s\tlast seen %s\n", device.ID, terminalText(device.Name), device.PairedAt.Local().Format(time.RFC3339), last)
		}
		return nil
	}
	if len(args) == 2 && args[0] == "revoke" {
		return admin.request(http.MethodPost, web.APIRoot+"/admin/devices/revoke/"+url.PathEscape(args[1]), nil, nil)
	}
	return fmt.Errorf("usage: surf devices list | surf devices revoke <device-id>")
}

type logSource struct {
	label string
	path  string
}

func renderLogRecords(data []byte) (string, error) {
	records, err := logstore.DecodeRecords(data)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, record := range records {
		fmt.Fprintf(&output, "%s %-5s %s  %s", record.Timestamp, strings.ToUpper(record.Level), record.Component, record.Message)
		keys := make([]string, 0, len(record.Fields))
		for key := range record.Fields {
			if !strings.Contains(record.Message, key+"=") {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			value, _ := json.Marshal(record.Fields[key])
			fmt.Fprintf(&output, "  %s=%s", key, value)
		}
		output.WriteByte('\n')
	}
	return output.String(), nil
}

func runLogsCommand(args []string) error {
	source, deviceID, follow := "all", "", false
	for len(args) > 0 {
		switch args[0] {
		case "--source":
			if len(args) < 2 {
				return fmt.Errorf("usage: surf logs [--source all|server|desktop|device] [--device ID] [--follow]")
			}
			source, args = args[1], args[2:]
		case "--device":
			if len(args) < 2 {
				return fmt.Errorf("usage: surf logs [--source all|server|desktop|device] [--device ID] [--follow]")
			}
			deviceID, args = args[1], args[2:]
		case "--follow":
			follow, args = true, args[1:]
		default:
			return fmt.Errorf("usage: surf logs [--source all|server|desktop|device] [--device ID] [--follow]")
		}
	}
	if source != "all" && source != "server" && source != "desktop" && source != "device" {
		return fmt.Errorf("unknown log source %q", source)
	}
	if source == "device" && deviceID == "" {
		return errors.New("--device is required with --source device")
	}
	admin, err := newLocalAdmin()
	if err != nil {
		return err
	}
	var sources struct {
		Devices []auth.Device `json:"devices"`
	}
	if source == "all" || source == "device" {
		if err := admin.request(http.MethodGet, web.APIRoot+"/admin/logs/sources", nil, &sources); err != nil {
			return err
		}
		if source == "device" {
			found := false
			for _, device := range sources.Devices {
				if device.ID == deviceID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("paired device %q was not found", deviceID)
			}
		}
		_, _ = admin.requestBytes(http.MethodPost, web.APIRoot+"/admin/logs/refresh", map[string]string{"deviceID": deviceID})
		time.Sleep(500 * time.Millisecond)
	}
	buildSources := func() []logSource {
		var result []logSource
		if source == "all" || source == "server" {
			result = append(result, logSource{label: "server", path: web.APIRoot + "/admin/logs?source=server"})
		}
		if source == "all" || source == "desktop" {
			result = append(result, logSource{label: "desktop", path: web.APIRoot + "/admin/logs?source=desktop"})
		}
		for _, device := range sources.Devices {
			if source == "device" && device.ID != deviceID || source != "all" && source != "device" {
				continue
			}
			result = append(result, logSource{label: "device " + terminalText(device.Name), path: web.APIRoot + "/admin/logs?source=device&deviceID=" + url.QueryEscape(device.ID)})
		}
		return result
	}
	previous := map[string]string{}
	printLogs := func(initial bool) error {
		for _, item := range buildSources() {
			data, err := admin.requestBytes(http.MethodGet, item.path, nil)
			if err != nil {
				return err
			}
			text := string(data)
			if initial {
				if text != "" {
					rendered, renderErr := renderLogRecords(data)
					if renderErr != nil {
						return renderErr
					}
					fmt.Printf("== %s ==\n%s", item.label, rendered)
				}
			} else if old := previous[item.path]; strings.HasPrefix(text, old) {
				rendered, renderErr := renderLogRecords([]byte(strings.TrimPrefix(text, old)))
				if renderErr != nil {
					return renderErr
				}
				fmt.Print(rendered)
			} else if text != old {
				rendered, renderErr := renderLogRecords(data)
				if renderErr != nil {
					return renderErr
				}
				fmt.Printf("\n== %s (rotated) ==\n%s", item.label, rendered)
			}
			previous[item.path] = text
		}
		return nil
	}
	if err := printLogs(true); err != nil || !follow {
		return err
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	ctx, cancel := signalContext()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := printLogs(false); err != nil {
				return err
			}
		}
	}
}

type clipboardCommand struct {
	action   string
	deviceID string
	enabled  bool
}

func parseClipboardCommand(args []string) (clipboardCommand, error) {
	usage := errors.New("usage: surf clipboard get | set [--device ID] | status | sync on|off")
	if len(args) == 1 && (args[0] == "get" || args[0] == "status") {
		return clipboardCommand{action: args[0]}, nil
	}
	if len(args) == 1 && args[0] == "set" {
		return clipboardCommand{action: "set"}, nil
	}
	if len(args) == 3 && args[0] == "set" && args[1] == "--device" && args[2] != "" {
		return clipboardCommand{action: "set", deviceID: args[2]}, nil
	}
	if len(args) == 2 && args[0] == "sync" && (args[1] == "on" || args[1] == "off") {
		return clipboardCommand{action: "sync", enabled: args[1] == "on"}, nil
	}
	return clipboardCommand{}, usage
}

func readClipboardText(reader io.Reader, terminalInput bool, readSecret func() ([]byte, error)) (string, error) {
	var data []byte
	var err error
	if terminalInput {
		data, err = readSecret()
	} else {
		data, err = io.ReadAll(io.LimitReader(reader, (64<<10)+1))
	}
	if err != nil {
		return "", err
	}
	if err := clipboard.Validate(string(data)); err != nil {
		return "", err
	}
	return string(data), nil
}

func runClipboardCommand(args []string) error {
	command, err := parseClipboardCommand(args)
	if err != nil {
		return err
	}
	admin, err := newLocalAdmin()
	if err != nil {
		return err
	}
	type clipboardState struct {
		Enabled   bool   `json:"enabled"`
		Available bool   `json:"available"`
		System    string `json:"system"`
		Known     bool   `json:"known"`
		Text      string `json:"text"`
		Error     string `json:"error"`
	}
	switch command.action {
	case "get":
		var state clipboardState
		if err := admin.request(http.MethodGet, web.APIRoot+"/admin/clipboard", nil, &state); err != nil {
			return err
		}
		if !state.Known {
			if state.Error != "" {
				return errors.New(state.Error)
			}
			return errors.New("clipboard has no text value")
		}
		_, err := io.WriteString(os.Stdout, state.Text)
		if err == nil && term.IsTerminal(int(os.Stdout.Fd())) && !strings.HasSuffix(state.Text, "\n") {
			fmt.Fprintln(os.Stdout)
		}
		return err
	case "status":
		var state clipboardState
		if err := admin.request(http.MethodGet, web.APIRoot+"/admin/clipboard", nil, &state); err != nil {
			return err
		}
		fmt.Printf("Two-way sync: %s\n", map[bool]string{true: "on", false: "off"}[state.Enabled])
		fmt.Printf("Host integration: %s", terminalText(state.System))
		if !state.Available {
			fmt.Print(" (unavailable)")
		}
		fmt.Println()
		if state.Error != "" {
			fmt.Printf("Host error: %s\n", terminalText(state.Error))
		}
		return nil
	case "sync":
		var state clipboardState
		if err := admin.request(http.MethodPut, web.APIRoot+"/admin/clipboard/sync",
			map[string]bool{"enabled": command.enabled}, &state); err != nil {
			return err
		}
		fmt.Printf("Two-way clipboard sync %s.\n", map[bool]string{true: "enabled", false: "disabled"}[state.Enabled])
		if state.Error != "" {
			fmt.Fprintf(os.Stderr, "surf: host clipboard warning: %s\n", terminalText(state.Error))
		}
		return nil
	}

	isTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	if isTerminal {
		fmt.Fprint(os.Stderr, "Clipboard text (input hidden; empty clears): ")
	}
	text, err := readClipboardText(os.Stdin, isTerminal, func() ([]byte, error) {
		data, readErr := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return data, readErr
	})
	if err != nil {
		return err
	}
	var result struct {
		OK        bool     `json:"ok"`
		Delivered []string `json:"delivered"`
		Failed    []string `json:"failed"`
		HostOK    bool     `json:"hostOK"`
		HostError string   `json:"hostError"`
	}
	if err := admin.request(http.MethodPost, web.APIRoot+"/admin/clipboard",
		map[string]string{"deviceID": command.deviceID, "text": text}, &result); err != nil {
		return err
	}
	if command.deviceID == "" {
		fmt.Printf("Clipboard set; synchronized with %d connected device(s).\n", len(result.Delivered))
	} else {
		fmt.Printf("Clipboard sent to %d connected device(s).\n", len(result.Delivered))
	}
	if result.HostError != "" {
		fmt.Fprintf(os.Stderr, "surf: host clipboard warning: %s\n", terminalText(result.HostError))
	}
	if !result.OK {
		return fmt.Errorf("clipboard delivery failed for %d device(s)", len(result.Failed))
	}
	return nil
}

func terminalText(value string) string {
	value = strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}
