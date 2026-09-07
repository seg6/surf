# Desktop interface icons

Original, unmodified SVGs from `lucide-static` **1.34.0**, retrieved from
`https://unpkg.com/lucide-static@1.34.0/icons/<name>.svg`.
ISC/MIT notices: `client/ios/Artwork/LUCIDE-LICENSE.txt` at the repository root.

Keep the upstream 24 × 24 viewBox and artwork padding. The desktop renders these
into a cached atlas at the current display scale, then centers the whole SVG
canvas in each control. Do not substitute text glyphs or trim individual icons
to their ink bounds: both change alignment and relative sizing.

The atlas is tinted using ImGui's current text color, including disabled/fade
alpha. Rasterization runs only on startup and display-scale changes, not per
frame. No network or system icon font is needed at runtime.
