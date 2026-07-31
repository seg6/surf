// Package auth owns paired devices, server-initiated pairing, signed sessions,
// and short-lived WebSocket tickets. Surf has no shared-password path.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"surf-backend/internal/atomicfile"
	"surf-backend/internal/statefile"
)

const (
	CookieName         = "surf_device"
	SessionTTL         = 24 * time.Hour
	ChallengeTTL       = 30 * time.Second
	WSTicketTTL        = 30 * time.Second
	CompletedPairTTL   = 10 * time.Minute
	MaxRateLimitPeers  = 4096
	MaxPairingAttempts = 5
)

var (
	ErrPairingClosed    = errors.New("the server owner has not started pairing")
	ErrPairingCode      = errors.New("the pairing code is incorrect")
	ErrPairingLocked    = errors.New("too many incorrect codes; start a new pairing session")
	ErrPairingUsed      = errors.New("this pairing code has already been used")
	ErrPairingNotFound  = errors.New("pairing request not found")
	ErrPairingComplete  = errors.New("pairing request is already complete")
	ErrAlreadyPaired    = errors.New("device is already paired")
	ErrChallengeExpired = errors.New("authentication challenge expired")
	ErrUnknownDevice    = errors.New("unknown device")
	ErrBadSignature     = errors.New("invalid device signature")
)

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey string    `json:"publicKey"`
	PairedAt  time.Time `json:"pairedAt"`
	LastSeen  time.Time `json:"lastSeen,omitempty"`
}

type PairingStatus struct {
	ID              string    `json:"id"`
	DeviceID        string    `json:"deviceID"`
	DeviceName      string    `json:"deviceName"`
	Phrase          string    `json:"phrase"`
	RequestedAt     time.Time `json:"requestedAt"`
	ClientConfirmed bool      `json:"clientConfirmed"`
	ServerApproved  bool      `json:"serverApproved"`
	Paired          bool      `json:"paired"`
}

type pairingCandidate struct {
	PairingStatus
	publicKey   string
	sourceIP    string
	sessionID   string
	completedAt time.Time
}

type PairingSession struct {
	ID          string    `json:"id"`
	Code        string    `json:"code,omitempty"`
	QRToken     string    `json:"qrToken,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Attempts    int       `json:"attempts"`
	CandidateID string    `json:"candidateID,omitempty"`
}

type rateWindow struct {
	hits []time.Time
	last time.Time
}

type Challenge struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"deviceID"`
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type wsTicket struct {
	deviceID string
	expires  time.Time
}

type Manager struct {
	mu sync.Mutex

	home       string
	serverID   string
	secret     []byte
	devices    map[string]*Device
	candidates map[string]*pairingCandidate
	challenges map[string]Challenge
	tickets    map[string]wsTicket
	pairing    *PairingSession

	pairAttempts map[string]*rateWindow
	authAttempts map[string]*rateWindow

	onRevoke func(string)
}

type registryFile struct {
	Devices []*Device `json:"devices"`
}

func New(home, serverID string) (*Manager, error) {
	secret, err := readOrCreateSecret(filepath.Join(home, "identity", "session.key"), 32)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		home: home, serverID: serverID, secret: secret,
		devices: map[string]*Device{}, candidates: map[string]*pairingCandidate{},
		challenges: map[string]Challenge{}, tickets: map[string]wsTicket{},
		pairAttempts: map[string]*rateWindow{}, authAttempts: map[string]*rateWindow{},
	}
	if err := m.loadDevices(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) ServerID() string { return m.serverID }

func (m *Manager) SetRevokeHandler(fn func(string)) {
	m.mu.Lock()
	m.onRevoke = fn
	m.mu.Unlock()
}

func (m *Manager) OpenPairing() PairingSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, candidate := range m.candidates {
		if !candidate.Paired {
			delete(m.candidates, id)
		}
	}
	session := PairingSession{
		ID: randomHex(16), Code: randomPairingCode(),
		QRToken:   base64.RawURLEncoding.EncodeToString(randomBytes(16)),
		CreatedAt: time.Now().UTC(),
	}
	m.pairing = &session
	return session
}

func (m *Manager) PairingSession() (PairingSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pairing == nil {
		return PairingSession{}, false
	}
	return *m.pairing, true
}

func (m *Manager) PairingAvailable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pairing != nil
}

func (m *Manager) ClosePairing() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pairing == nil {
		return false
	}
	if candidate := m.candidates[m.pairing.CandidateID]; candidate != nil && !candidate.Paired {
		delete(m.candidates, candidate.ID)
	}
	m.pairing = nil
	return true
}

func (m *Manager) DeviceCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.devices)
}

func (m *Manager) ListDevices() []Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Device, 0, len(m.devices))
	for _, device := range m.devices {
		out = append(out, *device)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PairedAt.Before(out[j].PairedAt) })
	return out
}

func (m *Manager) Revoke(deviceID string) bool {
	m.mu.Lock()
	device, ok := m.devices[deviceID]
	if !ok {
		m.mu.Unlock()
		return false
	}
	delete(m.devices, deviceID)
	for id, challenge := range m.challenges {
		if challenge.DeviceID == deviceID {
			delete(m.challenges, id)
		}
	}
	for ticket, value := range m.tickets {
		if value.deviceID == deviceID {
			delete(m.tickets, ticket)
		}
	}
	callback := m.onRevoke
	err := m.saveDevicesLocked()
	if err != nil {
		m.devices[deviceID] = device
	}
	m.mu.Unlock()
	if err == nil && callback != nil {
		callback(deviceID)
	}
	return err == nil
}

func (m *Manager) RequestPair(deviceName, publicKey, remoteAddr, code, qrToken string) (PairingStatus, error) {
	der, key, err := parsePublicKey(publicKey)
	if err != nil {
		return PairingStatus{}, err
	}
	if key.N.BitLen() != 2048 {
		return PairingStatus{}, fmt.Errorf("device key must be RSA-2048")
	}
	deviceSum := sha256.Sum256(der)
	deviceID := hex.EncodeToString(deviceSum[:])
	deviceName = sanitizeDeviceName(deviceName)
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	if m.pairing == nil {
		return PairingStatus{}, ErrPairingClosed
	}
	credentialMatches := (code != "" && qrToken == "" && hmac.Equal([]byte(code), []byte(m.pairing.Code))) ||
		(qrToken != "" && code == "" && hmac.Equal([]byte(qrToken), []byte(m.pairing.QRToken)))
	if !credentialMatches {
		m.pairing.Attempts++
		if m.pairing.Attempts >= MaxPairingAttempts {
			if candidate := m.candidates[m.pairing.CandidateID]; candidate != nil && !candidate.Paired {
				delete(m.candidates, candidate.ID)
			}
			m.pairing = nil
			return PairingStatus{}, ErrPairingLocked
		}
		return PairingStatus{}, ErrPairingCode
	}
	if m.pairing.CandidateID != "" {
		candidate := m.candidates[m.pairing.CandidateID]
		if candidate != nil && candidate.DeviceID == deviceID {
			candidate.DeviceName = deviceName
			return candidate.PairingStatus, nil
		}
		return PairingStatus{}, ErrPairingUsed
	}
	if m.devices[deviceID] != nil {
		return PairingStatus{}, ErrAlreadyPaired
	}
	sourceIP := normalizeIP(remoteAddr)
	id := randomHex(16)
	status := PairingStatus{
		ID: id, DeviceID: deviceID, DeviceName: deviceName,
		Phrase: pairingPhrase(m.serverID, der), RequestedAt: now.UTC(), ServerApproved: true,
	}
	m.candidates[id] = &pairingCandidate{
		PairingStatus: status, publicKey: publicKey, sourceIP: sourceIP, sessionID: m.pairing.ID,
	}
	m.pairing.CandidateID = id
	return status, nil
}

func (m *Manager) ConfirmPair(id string) (PairingStatus, error) {
	return m.updatePair(id)
}

func (m *Manager) RejectPair(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate, ok := m.candidates[id]
	if !ok || candidate.Paired {
		return false
	}
	delete(m.candidates, id)
	if m.pairing != nil && candidate.sessionID == m.pairing.ID {
		m.pairing = nil
	}
	return true
}

func (m *Manager) AckPair(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate, ok := m.candidates[id]
	if !ok || !candidate.Paired {
		return false
	}
	delete(m.candidates, id)
	return true
}

func (m *Manager) PairStatus(id string) (PairingStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	candidate, ok := m.candidates[id]
	if !ok {
		return PairingStatus{}, false
	}
	return candidate.PairingStatus, true
}

func (m *Manager) ListCandidates() []PairingStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	out := make([]PairingStatus, 0, len(m.candidates))
	for _, candidate := range m.candidates {
		out = append(out, candidate.PairingStatus)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt.Before(out[j].RequestedAt) })
	return out
}

func (m *Manager) updatePair(id string) (PairingStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	candidate := m.candidates[id]
	if candidate == nil {
		return PairingStatus{}, ErrPairingNotFound
	}
	candidate.ClientConfirmed = true
	if candidate.ClientConfirmed && candidate.ServerApproved && !candidate.Paired {
		candidate.Paired = true
		candidate.completedAt = time.Now()
		m.devices[candidate.DeviceID] = &Device{
			ID: candidate.DeviceID, Name: candidate.DeviceName, PublicKey: candidate.publicKey,
			PairedAt: time.Now().UTC(),
		}
		if err := m.saveDevicesLocked(); err != nil {
			candidate.Paired = false
			delete(m.devices, candidate.DeviceID)
			return PairingStatus{}, err
		}
		if m.pairing != nil && candidate.sessionID == m.pairing.ID {
			m.pairing = nil
		}
	}
	return candidate.PairingStatus, nil
}

func (m *Manager) NewChallenge(deviceID string) (Challenge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	if m.devices[deviceID] == nil {
		return Challenge{}, ErrUnknownDevice
	}
	challenge := Challenge{
		ID: randomHex(16), DeviceID: deviceID,
		Nonce:     base64.RawURLEncoding.EncodeToString(randomBytes(32)),
		ExpiresAt: time.Now().Add(ChallengeTTL),
	}
	m.challenges[challenge.ID] = challenge
	return challenge, nil
}

func (m *Manager) CompleteChallenge(challengeID, signature string) (string, error) {
	m.mu.Lock()
	challenge, ok := m.challenges[challengeID]
	delete(m.challenges, challengeID)
	if !ok || time.Now().After(challenge.ExpiresAt) {
		m.mu.Unlock()
		return "", ErrChallengeExpired
	}
	device := m.devices[challenge.DeviceID]
	publicKey := ""
	if device != nil {
		publicKey = device.PublicKey
	}
	m.mu.Unlock()
	if device == nil {
		return "", ErrUnknownDevice
	}
	_, key, err := parsePublicKey(publicKey)
	if err != nil {
		return "", err
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return "", ErrBadSignature
	}
	digest := authDigest(m.serverID, challenge)
	if err := rsa.VerifyPKCS1v15(key, cryptoHashSHA256, digest[:], sig); err != nil {
		return "", ErrBadSignature
	}
	m.mu.Lock()
	current := m.devices[challenge.DeviceID]
	if current == nil || current.PublicKey != publicKey {
		m.mu.Unlock()
		return "", ErrUnknownDevice
	}
	for id, candidate := range m.candidates {
		if candidate.DeviceID == challenge.DeviceID && candidate.Paired {
			delete(m.candidates, id)
		}
	}
	m.mu.Unlock()
	return challenge.DeviceID, nil
}

// cryptoHashSHA256 is the crypto.Hash value without importing crypto only for
// a single constant in old cross-compiled toolchains.
const cryptoHashSHA256 = 5

func authDigest(serverID string, challenge Challenge) [32]byte {
	value := "SURF-AUTH-V1\x00" + serverID + "\x00" + challenge.DeviceID + "\x00" + challenge.ID + "\x00" + challenge.Nonce
	return sha256.Sum256([]byte(value))
}

func (m *Manager) SetCookie(w http.ResponseWriter, deviceID string) {
	exp := time.Now().Add(SessionTTL).Unix()
	value := fmt.Sprintf("v1.%s.%d", deviceID, exp)
	value += "." + m.sign(value)
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: value, Path: "/api/v1/", MaxAge: int(SessionTTL.Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Path: "/api/v1/", MaxAge: -1, Secure: true, HttpOnly: true})
}

func (m *Manager) Valid(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 4 || parts[0] != "v1" {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	unsigned := strings.Join(parts[:3], ".")
	if err != nil || exp < time.Now().Unix() || !hmac.Equal([]byte(parts[3]), []byte(m.sign(unsigned))) {
		return "", false
	}
	m.mu.Lock()
	device := m.devices[parts[1]]
	if device != nil && time.Since(device.LastSeen) > time.Minute {
		device.LastSeen = time.Now().UTC()
		_ = m.saveDevicesLocked()
	}
	m.mu.Unlock()
	return parts[1], device != nil
}

func (m *Manager) WSTicket(deviceID string) string {
	ticket := randomHex(32)
	m.mu.Lock()
	m.tickets[ticket] = wsTicket{deviceID: deviceID, expires: time.Now().Add(WSTicketTTL)}
	m.mu.Unlock()
	return ticket
}

func (m *Manager) VerifyWSTicket(ticket string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	value, ok := m.tickets[ticket]
	delete(m.tickets, ticket)
	if !ok || m.devices[value.deviceID] == nil {
		return "", false
	}
	return value.deviceID, true
}

func (m *Manager) AllowPair(remoteAddr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return allowRate(m.pairAttempts, normalizeIP(remoteAddr), 10, time.Now())
}

func (m *Manager) AllowAuth(remoteAddr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return allowRate(m.authAttempts, normalizeIP(remoteAddr), 60, time.Now())
}

func allowRate(windows map[string]*rateWindow, ip string, limit int, now time.Time) bool {
	cutoff := now.Add(-time.Minute)
	for key, window := range windows {
		if window.last.Before(cutoff) {
			delete(windows, key)
		}
	}
	window := windows[ip]
	if window == nil {
		if len(windows) >= MaxRateLimitPeers {
			return false
		}
		window = &rateWindow{}
		windows[ip] = window
	}
	kept := window.hits[:0]
	for _, hit := range window.hits {
		if hit.After(cutoff) {
			kept = append(kept, hit)
		}
	}
	window.hits = kept
	window.last = now
	if len(window.hits) >= limit {
		return false
	}
	window.hits = append(window.hits, now)
	return true
}

func normalizeIP(remoteAddr string) string {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}
	if parsed := net.ParseIP(strings.Trim(ip, "[]")); parsed != nil {
		return parsed.String()
	}
	return "unknown"
}

func sanitizeDeviceName(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 80 {
		value = strings.TrimSpace(string(runes[:80]))
	}
	if value == "" {
		return "Surf device"
	}
	return value
}

func (m *Manager) sign(value string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func (m *Manager) pruneLocked(now time.Time) {
	for id, candidate := range m.candidates {
		if candidate.Paired && !candidate.completedAt.IsZero() && now.Sub(candidate.completedAt) > CompletedPairTTL {
			delete(m.candidates, id)
		}
	}
	for id, challenge := range m.challenges {
		if now.After(challenge.ExpiresAt) {
			delete(m.challenges, id)
		}
	}
	for id, ticket := range m.tickets {
		if now.After(ticket.expires) {
			delete(m.tickets, id)
		}
	}
}

func (m *Manager) loadDevices() error {
	path := filepath.Join(m.home, "devices.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read devices: %w", err)
	}
	var registry registryFile
	if err := json.Unmarshal(data, &registry); err != nil {
		backup, backupErr := statefile.Quarantine(path, "invalid")
		if backupErr != nil {
			return fmt.Errorf("parse devices: %w (quarantine failed: %v)", err, backupErr)
		}
		log.Printf("auth: ignored invalid device registry; backup: %s (%v)", backup, err)
		return nil
	}
	for _, device := range registry.Devices {
		if device != nil && device.ID != "" {
			m.devices[device.ID] = device
		}
	}
	return nil
}

func (m *Manager) saveDevicesLocked() error {
	registry := registryFile{Devices: make([]*Device, 0, len(m.devices))}
	for _, device := range m.devices {
		copy := *device
		registry.Devices = append(registry.Devices, &copy)
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path, tmp := filepath.Join(m.home, "devices.json"), filepath.Join(m.home, "devices.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o600)
	if err := atomicfile.Replace(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func parsePublicKey(encoded string) ([]byte, *rsa.PublicKey, error) {
	der, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("decode device public key: %w", err)
	}
	if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return der, key, nil
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, nil, fmt.Errorf("parse device public key: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("device key is not RSA")
	}
	return der, key, nil
}

var phraseWords = [...]string{
	"amber", "anchor", "apple", "april", "arrow", "atlas", "bamboo", "beacon",
	"birch", "bloom", "blue", "bridge", "brook", "canyon", "cedar", "cloud",
	"coral", "crane", "dawn", "delta", "drift", "eagle", "ember", "fern",
	"field", "finch", "forest", "frost", "garden", "glade", "gold", "harbor",
	"hazel", "heron", "island", "jade", "lake", "leaf", "lilac", "luna",
	"maple", "meadow", "mist", "moon", "north", "ocean", "olive", "orchid",
	"pearl", "pine", "quartz", "rain", "reed", "river", "robin", "sage",
	"shore", "silver", "sky", "stone", "sun", "tide", "willow", "wren",
}

func pairingPhrase(serverID string, publicDER []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte("SURF-PAIR-V1\x00"))
	_, _ = h.Write([]byte(serverID))
	_, _ = h.Write(publicDER)
	sum := h.Sum(nil)
	values := []byte{
		sum[0] >> 2,
		((sum[0] & 3) << 4) | (sum[1] >> 4),
		((sum[1] & 15) << 2) | (sum[2] >> 6),
		sum[2] & 63,
		sum[3] >> 2,
		((sum[3] & 3) << 4) | (sum[4] >> 4),
	}
	words := make([]string, len(values))
	for i, value := range values {
		words[i] = phraseWords[value]
	}
	return strings.Join(words, " ")
}

func readOrCreateSecret(path string, size int) ([]byte, error) {
	if value, err := os.ReadFile(path); err == nil && len(value) == size {
		return value, nil
	}
	value := randomBytes(size)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, value, 0o600); err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return value, nil
}

func randomBytes(size int) []byte {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return value
}

func randomHex(size int) string { return hex.EncodeToString(randomBytes(size)) }

func randomPairingCode() string {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", value.Int64())
}
