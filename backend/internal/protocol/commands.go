package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type Command interface {
	Kind() string
	Causal() (interactionID, clientNS uint64)
}

type CommandBase struct {
	T        string `json:"t"`
	IID      uint64 `json:"iid,omitempty"`
	ClientNS uint64 `json:"clientNs,omitempty"`
}

func (c CommandBase) Kind() string             { return c.T }
func (c CommandBase) Causal() (uint64, uint64) { return c.IID, c.ClientNS }

type EmptyCommand struct{ CommandBase }
type SizeCommand struct {
	CommandBase
	W int `json:"w"`
	H int `json:"h"`
}
type ClockCommand struct {
	CommandBase
	ClientSendNS uint64 `json:"c0"`
}
type TabCommand struct {
	CommandBase
	Action string `json:"action"`
	ID     int    `json:"id"`
}
type URLCommand struct {
	CommandBase
	URL string `json:"url"`
}
type ToggleCommand struct {
	CommandBase
	On bool `json:"on"`
}
type TouchPoint struct {
	ID      int     `json:"id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	RadiusX float64 `json:"rx,omitempty"`
	RadiusY float64 `json:"ry,omitempty"`
	Force   float64 `json:"force,omitempty"`
}

// TouchCommand carries physical contacts from UIKit: lifecycle edges contain
// changed contacts and move contains a complete active-contact snapshot.
// Coordinates and radii are fractions of the exact presented video surface;
// the browser input worker maps them through Chromium's CSS visual viewport.
type TouchCommand struct {
	CommandBase
	Phase       string       `json:"phase"`
	Sequence    uint64       `json:"seq"`
	Surface     uint32       `json:"surface"`
	TimestampNS uint64       `json:"ts"`
	Points      []TouchPoint `json:"points"`
}
type KeyCommand struct {
	CommandBase
	Down    bool   `json:"down"`
	Key     string `json:"key"`
	Code    string `json:"code"`
	KeyCode int    `json:"keyCode"`
	Text    string `json:"text"`
}
type TextCommand struct {
	CommandBase
	Text string `json:"text"`
}
type CompositionCommand struct {
	CommandBase
	Phase          string `json:"phase"`
	Text           string `json:"text"`
	SelectionStart int    `json:"start,omitempty"`
	SelectionEnd   int    `json:"end,omitempty"`
}
type QueryCommand struct {
	CommandBase
	Q      string `json:"q"`
	Offset int    `json:"offset"`
}
type FindCommand struct {
	CommandBase
	Q   string `json:"q"`
	Dir int    `json:"dir"`
}
type HistoryDeleteCommand struct {
	CommandBase
	URL string `json:"url"`
	TS  int64  `json:"ts"`
}
type NameCommand struct {
	CommandBase
	Name string `json:"name"`
}
type ClearCommand struct {
	CommandBase
	What string `json:"what"`
}
type DialogReplyCommand struct {
	CommandBase
	Accept bool   `json:"accept"`
	Text   string `json:"text"`
}
type MediaStatsCommand struct {
	CommandBase
	PresentedFPS float64 `json:"fps"`
	AURate       float64 `json:"auRate"`
	CallbackMS   float64 `json:"callbackMs"`
	GapMS        float64 `json:"gapMs"`
	DropPercent  float64 `json:"dropPct"`
	MemoryWarn   bool    `json:"memoryWarn"`
}
type VolumeCommand struct {
	CommandBase
	Value float64 `json:"value"`
}
type ClipboardResultCommand struct {
	CommandBase
	RequestID string `json:"id"`
	OK        bool   `json:"ok"`
}
type ClipboardChangeCommand struct {
	CommandBase
	Text string `json:"text"`
}
type LogRecordCommand struct {
	CommandBase
	Record json.RawMessage `json:"record"`
}
type LogClearedCommand struct{ CommandBase }

// DecodeCommand is the sole JSON ingress. Unknown commands and trailing JSON
// are rejected before browser state sees them.
func DecodeCommand(data []byte) (Command, error) {
	var header CommandBase
	if err := json.Unmarshal(data, &header); err != nil || header.T == "" {
		return nil, fmt.Errorf("invalid command envelope")
	}
	var dst Command
	switch header.T {
	case "size":
		dst = &SizeCommand{}
	case "tab":
		dst = &TabCommand{}
	case "nav", "opennew", "histdel", "bmdel":
		if header.T == "histdel" {
			dst = &HistoryDeleteCommand{}
		} else {
			dst = &URLCommand{}
		}
	case "audio", "mobile", "fullscreen":
		dst = &ToggleCommand{}
	case "touch":
		dst = &TouchCommand{}
	case "key":
		dst = &KeyCommand{}
	case "paste":
		dst = &TextCommand{}
	case "compose":
		dst = &CompositionCommand{}
	case "find":
		dst = &FindCommand{}
	case "suggest", "history":
		dst = &QueryCommand{}
	case "dldel":
		dst = &NameCommand{}
	case "clear":
		dst = &ClearCommand{}
	case "dialogreply":
		dst = &DialogReplyCommand{}
	case "clock":
		dst = &ClockCommand{}
	case "media-stats":
		dst = &MediaStatsCommand{}
	case "media-volume":
		dst = &VolumeCommand{}
	case "clipboard-result":
		dst = &ClipboardResultCommand{}
	case "clipboard-change":
		dst = &ClipboardChangeCommand{}
	case "log-record":
		dst = &LogRecordCommand{}
	case "log-cleared":
		dst = &LogClearedCommand{}
	case "back", "fwd", "reload", "stop", "video-retry", "reqkeyframe",
		"hist", "bookmark", "downloads", "reader", "media-playpause", "media-mute", "media-query":
		dst = &EmptyCommand{}
	default:
		return nil, fmt.Errorf("unknown command %q", header.T)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return nil, fmt.Errorf("decode %s: %w", header.T, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode %s: trailing JSON", header.T)
	}
	return dst, nil
}
