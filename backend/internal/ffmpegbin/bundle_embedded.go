//go:build ffmpeg_bundle

package ffmpegbin

import _ "embed"

//go:embed bundle/ffmpeg
var bundledFFmpeg []byte

func embeddedExecutable() []byte { return bundledFFmpeg }
