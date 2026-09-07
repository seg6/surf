//! Surf content and geometry over public ImGui widgets: IDs, navigation and activation
//! remain owned by ImGui, never by a parallel invisible-button interaction system.
use imgui::{StyleColor, StyleVar, Ui};

/// Public Dear ImGui placement API, missing a standalone wrapper in imgui-rs.
pub fn place_next(position: [f32; 2], size: [f32; 2]) {
    // SAFETY: called on the UI thread between NewFrame and Render.
    unsafe {
        imgui::sys::igSetNextWindowPos(
            position.into(),
            imgui::Condition::Always as i32,
            [0.0, 0.0].into(),
        );
        imgui::sys::igSetNextWindowSize(size.into(), imgui::Condition::Always as i32);
    }
}

pub fn icon_button(ui: &Ui, id: &str, glyph: &str, help: &str) -> bool {
    let _id = ui.push_id(id);
    let clicked = ui.button_with_size("###control", [crate::layout::CONTROL; 2]);
    centered_icon(ui, glyph, ui.item_rect_min(), ui.item_rect_max());
    if ui.is_item_hovered() {
        ui.tooltip_text(help);
    }
    clicked
}

/// Center the Lucide artwork, not its advance width or the surrounding text
/// font's baseline. Keep native Button/Selectable behavior; only draw its icon.
fn centered_icon(ui: &Ui, text: &str, min: [f32; 2], max: [f32; 2]) {
    let Some(codepoint) = text.chars().next() else {
        return;
    };
    // SAFETY: public ImGui APIs, on the UI thread in an active frame. The glyph
    // belongs to the current font atlas, which cannot change during this frame.
    let (glyph, color) = unsafe {
        let glyph = imgui::sys::ImFont_FindGlyphNoFallback(
            imgui::sys::igGetFont(),
            codepoint as imgui::sys::ImWchar,
        );
        let Some(glyph) = glyph.as_ref() else {
            return;
        };
        // Includes disabled-state and popup fade alpha, like native text.
        let color = imgui::sys::igGetColorU32_Col(StyleColor::Text as i32, 1.0);
        (glyph, color)
    };
    let scale = ui.current_font_size() / ui.current_font().font_size;
    let (a, b) = centered_artwork(min, max, [glyph.X1 - glyph.X0, glyph.Y1 - glyph.Y0], scale);
    // Use the existing font atlas quad directly: RenderText truncates the text
    // origin to whole logical pixels, which would undo fractional-DPI centering.
    ui.get_window_draw_list()
        .add_image(ui.fonts().tex_id, a, b)
        .uv_min([glyph.U0, glyph.V0])
        .uv_max([glyph.U1, glyph.V1])
        .col(color)
        .build();
}

fn centered_artwork(
    min: [f32; 2],
    max: [f32; 2],
    size: [f32; 2],
    scale: f32,
) -> ([f32; 2], [f32; 2]) {
    let center = [(min[0] + max[0]) * 0.5, (min[1] + max[1]) * 0.5];
    let half = [size[0] * scale * 0.5, size[1] * scale * 0.5];
    (
        [center[0] - half[0], center[1] - half[1]],
        [center[0] + half[0], center[1] + half[1]],
    )
}

pub fn section(ui: &Ui, title: &str) {
    ui.dummy([0.0, 8.0]);
    ui.text_disabled(title);
    ui.dummy([0.0, 2.0]);
}

pub fn sheet_header(ui: &Ui, title: &str, open: &mut bool) {
    let available = ui.content_region_avail()[0];
    let start = ui.cursor_pos();
    {
        let _font = ui.push_font(ui.fonts().fonts()[3]);
        ui.text(title);
    }
    ui.set_cursor_pos([start[0] + available - 30.0, start[1] - 4.0]);
    if icon_button(ui, "close-sheet", super::icon::CLOSE, "Close") {
        *open = false;
    }
    ui.set_cursor_pos([start[0], start[1] + 36.0]);
    ui.separator();
}

pub fn ellipsize(ui: &Ui, text: &str, width: f32) -> String {
    if ui.calc_text_size(text)[0] <= width {
        return text.to_owned();
    }
    let budget = (width - ui.calc_text_size("…")[0]).max(0.0);
    let mut result = String::new();
    for ch in text.chars() {
        result.push(ch);
        if ui.calc_text_size(&result)[0] > budget {
            result.pop();
            break;
        }
    }
    result.push('…');
    result
}

pub fn row(ui: &Ui, id: &str, title: &str, subtitle: &str, selected: bool, width: f32) -> bool {
    let _id = ui.push_id(id);
    let _pad = ui.push_style_var(StyleVar::ItemSpacing([4.0, 4.0]));
    let height = if subtitle.is_empty() { 30.0 } else { 44.0 };
    let clicked = ui
        .selectable_config("##row")
        .selected(selected)
        .close_popups(false)
        .size([width.max(1.0), height])
        .build();
    let p = ui.item_rect_min();
    let title = ellipsize(ui, title, width - 16.0);
    ui.get_window_draw_list().add_text(
        [
            p[0] + 8.0,
            p[1] + if subtitle.is_empty() { 8.0 } else { 5.0 },
        ],
        ui.style_color(StyleColor::Text),
        title,
    );
    if !subtitle.is_empty() {
        let _font = ui.push_font(ui.fonts().fonts()[2]);
        let subtitle = ellipsize(ui, subtitle, width - 16.0);
        ui.get_window_draw_list().add_text(
            [p[0] + 8.0, p[1] + 24.0],
            ui.style_color(StyleColor::TextDisabled),
            subtitle,
        );
    }
    clicked
}

pub fn menu_row(
    ui: &Ui,
    id: &str,
    glyph: &str,
    label: &str,
    shortcut: &str,
    checked: bool,
) -> bool {
    let width = ui.content_region_avail()[0];
    let _id = ui.push_id(id);
    let clicked = ui.selectable_config("##menu").size([width, 32.0]).build();
    let p = ui.item_rect_min();
    let bottom = ui.item_rect_max()[1];
    centered_icon(ui, glyph, p, [p[0] + 28.0, bottom]);
    let draw = ui.get_window_draw_list();
    let color = ui.style_color(StyleColor::Text);
    draw.add_text([p[0] + 32.0, p[1] + 8.0], color, label);
    let suffix = if checked { "✓" } else { shortcut };
    draw.add_text(
        [
            p[0] + width - ui.calc_text_size(suffix)[0] - 8.0,
            p[1] + 8.0,
        ],
        ui.style_color(StyleColor::TextDisabled),
        suffix,
    );
    clicked
}

#[cfg(test)]
mod tests {
    use super::centered_artwork;

    #[test]
    fn icon_artwork_is_centered_at_every_display_scale() {
        for dpi in [1.0, 1.25, 1.5, 2.0] {
            // Narrow ellipsis, asymmetric arrow bounds, and square plus/close.
            for size in [[4.0, 16.0], [13.0, 11.0], [16.0, 16.0]] {
                let raster_size = [size[0] * dpi, size[1] * dpi];
                let (min, max) = centered_artwork([6.0, 6.0], [36.0, 36.0], raster_size, 1.0 / dpi);
                for axis in 0..2 {
                    assert!(((min[axis] + max[axis]) * 0.5 - 21.0).abs() < 0.0001);
                    assert!((max[axis] - min[axis] - size[axis]).abs() < 0.0001);
                }
            }
        }
    }
}
