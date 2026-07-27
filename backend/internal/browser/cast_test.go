package browser

import (
	"testing"

	"surf-backend/internal/config"
	"surf-backend/internal/stream"
	"surf-backend/internal/ws"
)

// TestDesiredCastIgnoresMotionWithVideoSubscriber guards a real fix: with a
// video subscriber attached, motion-driven quality switching must not fire,
// because each switch was forcing ensureCast to fully stop+restart the CDP
// screencast — the H.264 lane's only frame source now — interrupting it.
// Confirmed live: real touch input (discrete swipes with pauses, unlike a
// perfectly continuous synthetic scroll) hit the motion/settle transition
// constantly, and the video feed barely updated as a result.
func TestDesiredCastIgnoresMotionWithVideoSubscriber(t *testing.T) {
	b := &Browser{
		cfg:   &config.Config{Quality: 100, MotionQuality: 85},
		viewW: 800, viewH: 600,
		videoSubs: map[*ws.Client]*stream.Sub{},
	}
	tab := &Tab{ID: 1, motion: true}

	// No video subscriber yet: motion should still drop quality, same as
	// before this fix — this behavior is unchanged for JPEG-only clients.
	q, w, h := b.desiredCastLocked(tab)
	if q != 85 || w != 800 || h != 600 {
		t.Fatalf("no subscriber: got q=%d %dx%d, want 85 800x600", q, w, h)
	}

	// A video subscriber attaches: motion must now be ignored and quality
	// stays at the cheaper video-source value so ensureCast never restarts
	// the cast just because the user is interacting.
	client := &ws.Client{}
	b.videoSubs[client] = &stream.Sub{}
	q, w, h = b.desiredCastLocked(tab)
	if q != 85 || w != 800 || h != 600 {
		t.Fatalf("with subscriber: got q=%d %dx%d, want 85 800x600 (stable video source)", q, w, h)
	}

	// Once the subscriber leaves, motion-based switching resumes.
	delete(b.videoSubs, client)
	q, _, _ = b.desiredCastLocked(tab)
	if q != 85 {
		t.Fatalf("after subscriber leaves: got q=%d, want 85 (motion switching resumed)", q)
	}
}
