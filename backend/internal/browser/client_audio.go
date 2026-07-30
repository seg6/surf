package browser

import (
	"log"
	"time"

	"surf-backend/internal/media"
	"surf-backend/internal/protocol"
	"surf-backend/internal/transport"
)

func (b *Controller) handleAudio(c *transport.Client, on bool) {
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
	c.SendJSON(protocol.AudioConfigEvent{Type: "audio-config", OK: true, Rate: 16000, Channels: 1})
	go b.pumpAudio(c, sub)
}

func (b *Controller) pumpAudio(c *transport.Client, sub *media.AudioSubscription) {
	for chunk := range sub.C {
		if !chunk.T.IsZero() {
			b.noteAudioLatency(time.Since(chunk.T))
		}
		_ = c.SendBinary(protocol.EncodeAudioPCM(chunk.Seq, chunk.SampleRate, chunk.Channels, chunk.Data))
	}
	c.SendJSON(protocol.AudioConfigEvent{Type: "audio-config"})
	b.stopAudio(c)
}

func (b *Controller) stopAudio(c *transport.Client) {
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
