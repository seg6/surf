package protocol

import "testing"

func TestDecodeCommandProducesConcreteType(t *testing.T) {
	command, err := DecodeCommand([]byte(`{"t":"scroll","phase":"move","x":0.5,"y":0.4,"dx":0,"dy":0.1,"iid":7,"clientNs":9}`))
	if err != nil {
		t.Fatal(err)
	}
	scroll, ok := command.(*ScrollCommand)
	if !ok || scroll.Phase != "move" || scroll.X != 0.5 || scroll.Y != 0.4 || scroll.DY != 0.1 {
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
		`{"t":"click","x":0.5,"y":0.5,"password":"secret"}`,
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
		`{"t":"bookmark"}`, `{"t":"clear","what":"cache"}`, `{"t":"click","x":0.1,"y":0.2}`,
		`{"t":"clock","c0":1}`, `{"t":"dldel","name":"x"}`, `{"t":"downloads"}`,
		`{"t":"find","q":"x","dir":1}`, `{"t":"fwd"}`, `{"t":"hist"}`,
		`{"t":"histdel","url":"https://example.test","ts":1}`, `{"t":"history","q":"x","offset":0}`,
		`{"t":"hit","x":0.1,"y":0.2}`, `{"t":"key","down":true,"key":"a","code":"KeyA","keyCode":65,"text":"a"}`,
		`{"t":"lpdown","x":0.1,"y":0.2}`, `{"t":"lpmove","x":0.1,"y":0.2}`,
		`{"t":"lpup","x":0.1,"y":0.2,"sel":true}`, `{"t":"nav","url":"https://example.test"}`,
		`{"t":"opennew","url":"https://example.test"}`, `{"t":"paste","text":"x"}`, `{"t":"reader"}`,
		`{"t":"reload"}`, `{"t":"reqkeyframe"}`, `{"t":"size","w":768,"h":934}`,
		`{"t":"suggest","q":"x","offset":0}`, `{"t":"tab","action":"select","id":1}`,
		`{"t":"scroll","phase":"begin","x":0.1,"y":0.2}`,
		`{"t":"scroll","phase":"move","x":0.1,"y":0.3,"dx":0,"dy":0.1}`,
		`{"t":"scroll","phase":"end"}`, `{"t":"zoom","scale":2,"cx":0.5,"cy":0.5}`,
		`{"t":"video-retry"}`, `{"t":"stop"}`, `{"t":"dialogreply","accept":true,"text":""}`,
		`{"t":"media-query"}`, `{"t":"media-playpause"}`, `{"t":"media-mute"}`,
		`{"t":"media-volume","value":0.5}`, `{"t":"mobile","on":true}`,
		`{"t":"fullscreen","on":true}`,
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
