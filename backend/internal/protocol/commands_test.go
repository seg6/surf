package protocol

import "testing"

func TestDecodeCommandProducesConcreteType(t *testing.T) {
	command, err := DecodeCommand([]byte(`{"t":"touch","phase":"move","seq":3,"surface":7,"ts":99,"points":[{"id":1,"x":0.5,"y":0.4,"rx":0.01,"ry":0.02,"force":0.5}],"iid":7,"clientNs":9}`))
	if err != nil {
		t.Fatal(err)
	}
	touch, ok := command.(*TouchCommand)
	if !ok || touch.Phase != "move" || touch.Sequence != 3 || touch.Surface != 7 ||
		len(touch.Points) != 1 || touch.Points[0].ID != 1 || touch.Points[0].X != 0.5 || touch.Points[0].Force != 0.5 {
		t.Fatalf("command = %#v", command)
	}
	iid, clientNS := command.Causal()
	if iid != 7 || clientNS != 9 {
		t.Fatalf("causal = %d %d", iid, clientNS)
	}
}

func TestDecodeCommandRejectsUnknownTypeAndFields(t *testing.T) {
	for _, data := range []string{
		`{"t":"wat"}`,
		`{"t":"touch","phase":"start","seq":1,"surface":1,"ts":1,"points":[],"password":"secret"}`,
		`{"t":"size","w":"wide","h":768}`,
	} {
		if _, err := DecodeCommand([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestDecodeEveryNativeCommand(t *testing.T) {
	cases := []string{
		`{"t":"audio","on":true}`, `{"t":"back"}`, `{"t":"bmdel","url":"https://example.test"}`,
		`{"t":"bookmark"}`, `{"t":"clear","what":"cache"}`,
		`{"t":"clock","c0":1}`, `{"t":"dldel","name":"x"}`, `{"t":"downloads"}`,
		`{"t":"find","q":"x","dir":1}`, `{"t":"fwd"}`, `{"t":"hist"}`,
		`{"t":"histdel","url":"https://example.test","ts":1}`, `{"t":"history","q":"x","offset":0}`,
		`{"t":"key","down":true,"key":"a","code":"KeyA","keyCode":65,"text":"a"}`,
		`{"t":"nav","url":"https://example.test"}`,
		`{"t":"opennew","url":"https://example.test"}`, `{"t":"paste","text":"x"}`, `{"t":"reader"}`,
		`{"t":"reload"}`, `{"t":"reqkeyframe"}`, `{"t":"size","w":768,"h":934}`,
		`{"t":"suggest","q":"x","offset":0}`, `{"t":"tab","action":"select","id":1}`,
		`{"t":"touch","phase":"start","seq":1,"surface":1,"ts":1,"points":[{"id":1,"x":0.1,"y":0.2}]}`,
		`{"t":"touch","phase":"move","seq":2,"surface":1,"ts":2,"points":[{"id":1,"x":0.1,"y":0.3}]}`,
		`{"t":"touch","phase":"end","seq":3,"surface":1,"ts":3,"points":[{"id":1,"x":0.1,"y":0.3}]}`,
		`{"t":"touch","phase":"cancel","seq":4,"surface":1,"ts":4,"points":[]}`,
		`{"t":"compose","phase":"update","text":"kan","start":3,"end":3}`,
		`{"t":"video-retry"}`, `{"t":"stop"}`, `{"t":"dialogreply","accept":true,"text":""}`,
		`{"t":"media-query"}`, `{"t":"media-playpause"}`, `{"t":"media-mute"}`,
		`{"t":"media-volume","value":0.5}`, `{"t":"mobile","on":true}`,
		`{"t":"fullscreen","on":true}`,
		`{"t":"clipboard-result","id":"request","ok":true}`,
		`{"t":"clipboard-change","text":"device clipboard"}`,
		`{"t":"log-record","record":{"ts":"now","level":"info","component":"app","message":"ready","fields":{}}}`,
		`{"t":"log-cleared"}`,
	}
	for _, data := range cases {
		command, err := DecodeCommand([]byte(data))
		if err != nil {
			t.Errorf("%s: %v", data, err)
		} else if command.Kind() == "" {
			t.Errorf("%s: empty kind", data)
		}
	}
}
