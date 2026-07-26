package browser

import (
	"log"
	"time"

	"surf-backend/internal/audio"
	"surf-backend/internal/protocol"
	"surf-backend/internal/ws"
)

func (b *Browser) handleAudio(c *ws.Client, on bool) {
	if !on {
		b.stopAudio(c)
		return
	}
	b.mediaMu.Lock()
	if _, exists := b.audioSubs[c]; exists {
		b.mediaMu.Unlock()
		return
	}
	sub := b.audio.Subscribe()
	b.audioSubs[c] = sub
	b.mediaMu.Unlock()
	log.Printf("audio: client subscribed")
	c.SendJSON(map[string]any{"t": "audio-config", "ok": true, "rate": 16000, "channels": 1})
	go b.pumpAudio(c, sub)
}

func (b *Browser) pumpAudio(c *ws.Client, sub *audio.Sub) {
	for chunk := range sub.C {
		if !chunk.T.IsZero() {
			b.noteAudioLatency(time.Since(chunk.T))
		}
		_ = c.SendBinary(protocol.EncodeAudioPCM(chunk.Seq, chunk.SampleRate, chunk.Channels, chunk.Data))
	}
	c.SendJSON(map[string]any{"t": "audio-config", "ok": false})
	b.stopAudio(c)
}

func (b *Browser) stopAudio(c *ws.Client) {
	b.mediaMu.Lock()
	sub := b.audioSubs[c]
	delete(b.audioSubs, c)
	b.mediaMu.Unlock()
	if sub == nil {
		return
	}
	sub.Close()
	log.Printf("audio: client unsubscribed")
}
