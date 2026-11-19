package browser

import (
	"testing"

	"surf-backend/internal/media"
	"surf-backend/internal/transport"
)

func TestPausedVideoClientIsNotResubscribed(t *testing.T) {
	b := &Controller{
		videoSubs:   map[*transport.Client]*media.VideoSubscription{},
		videoPaused: map[*transport.Client]bool{},
	}
	c := &transport.Client{}

	b.handleVideo(c, false)
	if !b.videoIsPaused(c) {
		t.Fatal("video was not marked paused")
	}
	// The controller has no video pipeline; reaching it would panic. A paused
	// client must return before any encoder or socket work.
	b.subscribeVideo(c)
	b.videoFailed(c)
	if _, subscribed := b.videoSubs[c]; subscribed {
		t.Fatal("paused client was subscribed")
	}

	b.mediaMu.Lock()
	delete(b.videoPaused, c)
	b.mediaMu.Unlock()
	if b.videoIsPaused(c) {
		t.Fatal("video still paused after clearing")
	}
}
