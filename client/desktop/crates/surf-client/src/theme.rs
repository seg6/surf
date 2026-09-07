use imgui::{Context, FontConfig, FontGlyphRanges, FontSource, StyleColor};

#[derive(Clone, Copy)]
pub struct Palette {
    pub canvas: [f32; 4],
    pub rail: [f32; 4],
    pub surface: [f32; 4],
    pub border: [f32; 4],
    pub text: [f32; 4],
    pub accent: [f32; 4],
    pub muted: [f32; 4],
    pub hover: [f32; 4],
}

const fn hex(rgb: u32) -> [f32; 4] {
    [
        ((rgb >> 16) & 255) as f32 / 255.0,
        ((rgb >> 8) & 255) as f32 / 255.0,
        (rgb & 255) as f32 / 255.0,
        1.0,
    ]
}

impl Palette {
    pub fn new(dark: bool) -> Self {
        if dark {
            Self {
                canvas: hex(0x101113),
                rail: hex(0x191b1e),
                surface: hex(0x24262b),
                border: hex(0x3b3e45),
                text: hex(0xf0f1f4),
                accent: hex(0x56a2ce),
                muted: hex(0xa3a7b0),
                hover: hex(0x34373d),
            }
        } else {
            Self {
                canvas: hex(0xf4f6f8),
                rail: hex(0xf8fafc),
                surface: hex(0xffffff),
                border: hex(0xd4dae0),
                text: hex(0x1f252b),
                accent: hex(0x1473b8),
                muted: hex(0x606975),
                hover: hex(0xe5e9ee),
            }
        }
    }
}

pub fn install_style(context: &mut Context) {
    let s = context.style_mut();
    s.window_padding = [12.0, 12.0];
    s.frame_padding = [8.0, 8.0];
    s.item_spacing = [4.0, 4.0];
    s.item_inner_spacing = [6.0, 4.0];
    s.scrollbar_size = 8.0;
    s.grab_min_size = 10.0;
    s.window_rounding = 8.0;
    s.child_rounding = 6.0;
    s.frame_rounding = 6.0;
    s.popup_rounding = 8.0;
    s.scrollbar_rounding = 6.0;
    s.tab_rounding = 6.0;
    s.window_border_size = 1.0;
    s.child_border_size = 0.0;
    s.frame_border_size = 0.0;
    apply_palette(s, true);
}

pub fn apply_palette(s: &mut imgui::Style, dark: bool) {
    use StyleColor::*;
    let p = Palette::new(dark);
    for (key, value) in [
        (Text, p.text),
        (TextDisabled, p.muted),
        (WindowBg, p.surface),
        (ChildBg, [0.0; 4]),
        (PopupBg, p.surface),
        (Border, p.border),
        (BorderShadow, [0.0; 4]),
        (FrameBg, p.rail),
        (FrameBgHovered, p.hover),
        (FrameBgActive, p.hover),
        (Button, p.rail),
        (ButtonHovered, p.hover),
        (ButtonActive, p.border),
        (Header, p.hover),
        (HeaderHovered, p.hover),
        (HeaderActive, p.border),
        (CheckMark, p.accent),
        (PlotHistogram, p.accent),
        (SliderGrab, p.accent),
        (SliderGrabActive, p.accent),
        (Separator, p.border),
        (SeparatorHovered, p.accent),
        (SeparatorActive, p.accent),
        (NavHighlight, p.accent),
        (
            TextSelectedBg,
            [p.accent[0], p.accent[1], p.accent[2], 0.35],
        ),
        (TitleBg, p.rail),
        (TitleBgActive, p.rail),
        (TitleBgCollapsed, p.rail),
        (ScrollbarBg, [0.0; 4]),
        (ScrollbarGrab, p.border),
        (ScrollbarGrabHovered, p.muted),
        (ScrollbarGrabActive, p.muted),
        (ResizeGrip, [0.0; 4]),
        (ResizeGripHovered, p.border),
        (ResizeGripActive, p.accent),
        (ModalWindowDimBg, [0.0, 0.0, 0.0, 0.45]),
    ] {
        s.colors[key as usize] = value;
    }
}

// Fonts are bundled; neither typography nor icons depends on installed desktop fonts.
pub fn install_fonts(context: &mut Context, scale: f32) {
    let fonts = context.fonts();
    fonts.clear();
    let regular = include_bytes!("../../../assets/fonts/Inter-Regular.ttf");
    let medium = include_bytes!("../../../assets/fonts/Inter-Medium.ttf");
    let semibold = include_bytes!("../../../assets/fonts/Inter-SemiBold.ttf");
    for (data, size) in [
        (regular.as_slice(), 14.0),
        (medium.as_slice(), 13.0),
        (regular.as_slice(), 12.0),
        (semibold.as_slice(), 18.0),
        (semibold.as_slice(), 24.0),
    ] {
        fonts.add_font(&[FontSource::TtfData {
            data,
            size_pixels: size * scale,
            config: Some(FontConfig {
                oversample_h: 2,
                oversample_v: 2,
                glyph_ranges: FontGlyphRanges::from_slice(&[
                    0x20, 0x24f, 0x370, 0x52f, 0x2000, 0x206f, 0x20a0, 0x214f, 0x2190, 0x27ff,
                    0xfffd, 0xfffd, 0,
                ]),
                ..FontConfig::default()
            }),
        }]);
    }
    context.io_mut().font_global_scale = 1.0 / scale;
}
