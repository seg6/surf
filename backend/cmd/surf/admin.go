package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"
	"unicode"

	"surf-backend/internal/auth"
	"surf-backend/internal/control"
	"surf-backend/internal/web"

	qrcode "github.com/skip2/go-qrcode"
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
		return nil, fmt.Errorf("Surf daemon is not running for SURF_HOME=%s: %w", home, err)
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
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, a.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set(control.AdminHeader, a.descriptor.AdminToken)
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("Surf daemon is not running at %s: %w", a.base, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("backend returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			return err
		}
	}
	return nil
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
		Name     string `json:"name"`
		ServerID string `json:"serverID"`
		Protocol string `json:"protocol"`
		Version  string `json:"version"`
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
	fmt.Println("Surf daemon is running.")
	fmt.Printf("Name: %s\n", terminalText(server.Name))
	fmt.Printf("PID: %d\n", admin.descriptor.PID)
	fmt.Printf("Port: %d\n", admin.descriptor.PublicPort)
	fmt.Printf("Version: %s\nProtocol: %s\n", server.Version, server.Protocol)
	fmt.Printf("Server ID: %s\n", server.ServerID)
	fmt.Printf("Paired devices: %d\n", len(devices.Devices))
	return nil
}

func runPairCommand() error {
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
	fmt.Println("On the client, scan this QR code. On older devices, enter the server address and six-digit code.")
	if address != "" {
		fmt.Printf("Manual address: %s\n", address)
		if code, err := qrcode.New(session.PairingURL, qrcode.Medium); err == nil {
			fmt.Print(code.ToSmallString(false))
		}
	}
	fmt.Println("Only this one-time code can pair one device. Press Ctrl+C to cancel it.")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	shown := map[string]bool{}
	for {
		select {
		case <-interrupt:
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

func terminalText(value string) string {
	value = strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}
