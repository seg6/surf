//go:build !ffmpeg_bundle

package ffmpegbin

func embeddedExecutable() []byte { return nil }
