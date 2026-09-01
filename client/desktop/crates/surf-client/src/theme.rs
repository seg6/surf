use eframe::egui::{self, Color32, Stroke, Vec2};

#[derive(Clone, Copy)]
pub struct Palette {
    pub background: Color32,
    pub chrome: Color32,
    pub command_rail: Color32,
    pub field: Color32,
    pub field_border: Color32,
    pub surface: Color32,
    pub tab_active: Color32,
    pub tab_border: Color32,
    pub separator: Color32,
    pub text: Color32,
    pub muted: Color32,
    pub disabled: Color32,
    pub accent: Color32,
    pub accent_soft: Color32,
    pub accent_text: Color32,
    pub danger: Color32,
}

const DARK: Palette = Palette {
    background: Color32::from_rgb(23, 23, 25),
    chrome: Color32::from_rgb(29, 29, 32),
    command_rail: Color32::from_rgb(32, 32, 35),
    field: Color32::from_rgb(42, 42, 46),
    field_border: Color32::from_rgb(68, 68, 74),
    surface: Color32::from_rgb(35, 35, 38),
    tab_active: Color32::from_rgb(46, 46, 50),
    tab_border: Color32::from_rgb(77, 77, 84),
    separator: Color32::from_rgb(55, 55, 60),
    text: Color32::from_rgb(244, 244, 245),
    muted: Color32::from_rgb(165, 165, 172),
    disabled: Color32::from_rgb(94, 94, 101),
    accent: Color32::from_rgb(90, 200, 216),
    accent_soft: Color32::from_rgb(37, 63, 67),
    accent_text: Color32::from_rgb(142, 220, 229),
    danger: Color32::from_rgb(241, 116, 116),
};

const LIGHT: Palette = Palette {
    background: Color32::from_rgb(239, 239, 241),
    chrome: Color32::from_rgb(250, 250, 251),
    command_rail: Color32::from_rgb(246, 246, 248),
    field: Color32::from_rgb(255, 255, 255),
    field_border: Color32::from_rgb(198, 198, 204),
    surface: Color32::from_rgb(255, 255, 255),
    tab_active: Color32::from_rgb(232, 232, 235),
    tab_border: Color32::from_rgb(184, 184, 191),
    separator: Color32::from_rgb(210, 210, 215),
    text: Color32::from_rgb(28, 28, 31),
    muted: Color32::from_rgb(94, 94, 101),
    disabled: Color32::from_rgb(164, 164, 171),
    accent: Color32::from_rgb(24, 139, 158),
    accent_soft: Color32::from_rgb(214, 241, 245),
    accent_text: Color32::from_rgb(14, 103, 119),
    danger: Color32::from_rgb(184, 54, 54),
};

pub fn palette(dark: bool) -> Palette {
    if dark { DARK } else { LIGHT }
}

pub fn apply(context: &egui::Context, dark: bool) {
    let colors = palette(dark);
    let preference = if dark {
        egui::ThemePreference::Dark
    } else {
        egui::ThemePreference::Light
    };
    context.set_theme(preference);
    let selected = if dark {
        egui::Theme::Dark
    } else {
        egui::Theme::Light
    };
    let mut style = (*context.style_of(selected)).clone();
    style.spacing.item_spacing = Vec2::new(8.0, 8.0);
    style.visuals.dark_mode = dark;
    style.visuals.window_fill = colors.surface;
    style.visuals.panel_fill = colors.background;
    style.visuals.extreme_bg_color = colors.field;
    style.visuals.override_text_color = Some(colors.text);
    style.visuals.selection.bg_fill = colors.accent;
    style.visuals.selection.stroke = Stroke::new(1.0, colors.accent_text);
    style.visuals.widgets.noninteractive.bg_fill = colors.surface;
    style.visuals.widgets.noninteractive.fg_stroke = Stroke::new(1.0, colors.text);
    style.visuals.widgets.inactive.bg_fill = colors.field;
    style.visuals.widgets.inactive.fg_stroke = Stroke::new(1.0, colors.text);
    style.visuals.widgets.hovered.bg_fill = colors.tab_active;
    style.visuals.widgets.hovered.fg_stroke = Stroke::new(1.0, colors.text);
    style.visuals.window_stroke = Stroke::new(1.0, colors.separator);
    context.set_style_of(selected, style);
}

#[cfg(test)]
mod tests {
    use super::palette;

    #[test]
    fn palettes_keep_text_and_controls_distinct() {
        for colors in [palette(false), palette(true)] {
            assert_ne!(colors.text, colors.background);
            assert_ne!(colors.text, colors.field);
            assert_ne!(colors.field, colors.field_border);
            assert_ne!(colors.tab_active, colors.chrome);
        }
    }
}
