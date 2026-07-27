# Third-party software

Surf release packages include an FFmpeg executable.

The Linux build is produced by the BtbN FFmpeg-Builds project and includes
GPL-licensed components, including libx264. Windows and macOS builds are from
the ffmpeg-static project. FFmpeg and its incorporated libraries remain under
their respective licenses; they are not covered by Surf's MIT license.

- FFmpeg source and license information: https://ffmpeg.org/
- BtbN build scripts and corresponding sources: https://github.com/BtbN/FFmpeg-Builds
- ffmpeg-static build and license information: https://github.com/eugeneware/ffmpeg-static
- x264 source: https://code.videolan.org/videolan/x264

The exact artifact versions and SHA-256 values used by Surf are recorded in
`backend/internal/ffmpegbin`.
