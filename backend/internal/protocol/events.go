package protocol

// Event is the complete set of JSON messages that the backend may send to a
// native client. Keeping the marker private prevents browser and transport
// code from accidentally putting an arbitrary map or CDP result on the wire.
type Event interface {
	isClientEvent()
}

type HelloEvent struct {
	Type string `json:"t"`
	W    int    `json:"vw"`
	H    int    `json:"vh"`
}

type TabsEvent struct {
	Type string    `json:"t"`
	Tabs []TabInfo `json:"tabs"`
}

type VideoConfigEvent struct {
	Type       string `json:"t"`
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	W          int    `json:"w,omitempty"`
	H          int    `json:"h,omitempty"`
	Generation uint32 `json:"generation,omitempty"`
	Profile    string `json:"profile,omitempty"`
}

type AudioConfigEvent struct {
	Type     string `json:"t"`
	OK       bool   `json:"ok"`
	Rate     int    `json:"rate,omitempty"`
	Channels int    `json:"channels,omitempty"`
}

type BoolEvent struct {
	Type string `json:"t"`
	On   bool   `json:"on"`
}

type TextEvent struct {
	Type string `json:"t"`
	Text string `json:"text"`
}

type ClockEvent struct {
	Type          string `json:"t"`
	ClientSendNS  uint64 `json:"c0"`
	BackendRecvNS uint64 `json:"s1"`
	BackendSendNS uint64 `json:"s2"`
}

type EmptyEvent struct {
	Type string `json:"t"`
}

type URLStateEvent struct {
	Type     string `json:"t"`
	URL      string `json:"url"`
	Starred  bool   `json:"starred"`
	Security string `json:"security,omitempty"`
}

type HistoryStateEvent struct {
	Type string `json:"t"`
	Back bool   `json:"back"`
	Fwd  bool   `json:"fwd"`
}

type NameEvent struct {
	Type string `json:"t"`
	Name string `json:"name"`
}

type DownloadProgressEvent struct {
	Type string `json:"t"`
	Name string `json:"name"`
	Pct  int    `json:"pct"`
}

type LibraryEntry struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	TS    int64  `json:"ts"`
}

type SuggestEvent struct {
	Type  string         `json:"t"`
	Items []LibraryEntry `json:"items"`
}

type LibraryEvent struct {
	Type      string         `json:"t"`
	History   []LibraryEntry `json:"hist"`
	Bookmarks []LibraryEntry `json:"bookmarks"`
	Starred   bool           `json:"starred"`
}

type HistoryPageEvent struct {
	Type   string         `json:"t"`
	Query  string         `json:"q,omitempty"`
	Items  []LibraryEntry `json:"items"`
	Offset int            `json:"offset"`
	Total  int            `json:"total"`
}

type DownloadItem struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	TS   int64  `json:"ts"`
}

type DownloadsEvent struct {
	Type  string         `json:"t"`
	Items []DownloadItem `json:"items"`
}

type DialogEvent struct {
	Type    string `json:"t"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	Default string `json:"def"`
}

type FileChooserEvent struct {
	Type     string `json:"t"`
	Multiple bool   `json:"multiple"`
}

type SecurityEvent struct {
	Type  string `json:"t"`
	State string `json:"state"`
}

type ReaderEvent struct {
	Type  string `json:"t"`
	OK    bool   `json:"ok"`
	Title string `json:"title,omitempty"`
	HTML  string `json:"html,omitempty"`
	URL   string `json:"url,omitempty"`
}

type EditableEvent struct {
	Type string    `json:"t"`
	On   bool      `json:"on"`
	Kind string    `json:"kind,omitempty"`
	Rect []float64 `json:"rect,omitempty"`
}

type MediaStateEvent struct {
	Type        string  `json:"t"`
	Available   bool    `json:"available"`
	Count       int     `json:"count"`
	Paused      bool    `json:"paused"`
	Muted       bool    `json:"muted"`
	Volume      float64 `json:"volume"`
	CurrentTime float64 `json:"currentTime"`
	Duration    float64 `json:"duration"`
	Title       string  `json:"title,omitempty"`
}

type PageFrameEvent struct {
	Type      string `json:"t"`
	SourceSeq uint32 `json:"sourceSeq"`
}

type ClipboardEvent struct {
	Type      string `json:"t"`
	RequestID string `json:"id"`
	Text      string `json:"text"`
	Sync      bool   `json:"sync,omitempty"`
}

type ClipboardSyncEvent struct {
	Type    string `json:"t"`
	Enabled bool   `json:"enabled"`
	Known   bool   `json:"known,omitempty"`
	Text    string `json:"text"`
}

func (HelloEvent) isClientEvent()            {}
func (TabsEvent) isClientEvent()             {}
func (VideoConfigEvent) isClientEvent()      {}
func (AudioConfigEvent) isClientEvent()      {}
func (BoolEvent) isClientEvent()             {}
func (TextEvent) isClientEvent()             {}
func (ClockEvent) isClientEvent()            {}
func (EmptyEvent) isClientEvent()            {}
func (URLStateEvent) isClientEvent()         {}
func (HistoryStateEvent) isClientEvent()     {}
func (NameEvent) isClientEvent()             {}
func (DownloadProgressEvent) isClientEvent() {}
func (SuggestEvent) isClientEvent()          {}
func (LibraryEvent) isClientEvent()          {}
func (HistoryPageEvent) isClientEvent()      {}
func (DownloadsEvent) isClientEvent()        {}
func (DialogEvent) isClientEvent()           {}
func (FileChooserEvent) isClientEvent()      {}
func (SecurityEvent) isClientEvent()         {}
func (ReaderEvent) isClientEvent()           {}
func (EditableEvent) isClientEvent()         {}
func (MediaStateEvent) isClientEvent()       {}
func (PageFrameEvent) isClientEvent()        {}
func (ClipboardEvent) isClientEvent()        {}
func (ClipboardSyncEvent) isClientEvent()    {}
