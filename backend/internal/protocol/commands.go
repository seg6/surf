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
type PointCommand struct {
	CommandBase
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type ScrollCommand struct {
	CommandBase
	Phase string  `json:"phase"`
	X     float64 `json:"x,omitempty"`
	Y     float64 `json:"y,omitempty"`
	DX    float64 `json:"dx,omitempty"`
	DY    float64 `json:"dy,omitempty"`
}
type LongPressUpCommand struct {
	CommandBase
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Sel bool    `json:"sel"`
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
type ZoomCommand struct {
	CommandBase
	Scale float64 `json:"scale"`
	CX    float64 `json:"cx"`
	CY    float64 `json:"cy"`
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
	case "click", "lpdown", "lpmove", "hit":
		dst = &PointCommand{}
	case "scroll":
		dst = &ScrollCommand{}
	case "lpup":
		dst = &LongPressUpCommand{}
	case "key":
		dst = &KeyCommand{}
	case "paste":
		dst = &TextCommand{}
	case "zoom":
		dst = &ZoomCommand{}
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
