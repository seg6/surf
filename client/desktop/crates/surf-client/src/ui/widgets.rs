//! Surf content and geometry over public ImGui widgets: IDs, navigation and activation
//! remain owned by ImGui, never by a parallel invisible-button interaction system.
use crate::icons::{Icon, IconAtlas};
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

pub fn icon_button(ui: &Ui, icons: &IconAtlas, id: &str, glyph: Icon, help: &str) -> bool {
    let _id = ui.push_id(id);
    let clicked = ui.button_with_size("###control", [crate::layout::CONTROL; 2]);
    icons.draw(ui, glyph, ui.item_rect_min(), ui.item_rect_max());
    if ui.is_item_hovered() {
        ui.tooltip_text(help);
    }
    clicked
}

pub fn section(ui: &Ui, title: &str) {
    ui.dummy([0.0, 12.0]);
    let _font = ui.push_font(ui.fonts().fonts()[1]);
    ui.text(title);
    ui.dummy([0.0, 4.0]);
}

pub fn description(ui: &Ui, text: &str) {
    let _color = ui.push_style_color(StyleColor::Text, ui.style_color(StyleColor::TextDisabled));
    ui.text_wrapped(text);
}

pub fn primary_button(ui: &Ui, label: &str, width: f32) -> bool {
    let _bg = ui.push_style_color(StyleColor::Button, [0.12, 0.36, 0.56, 1.0]);
    let _hover = ui.push_style_color(StyleColor::ButtonHovered, [0.16, 0.43, 0.66, 1.0]);
    let _active = ui.push_style_color(StyleColor::ButtonActive, [0.09, 0.29, 0.46, 1.0]);
    let _text = ui.push_style_color(StyleColor::Text, [1.0; 4]);
    ui.button_with_size(label, [width, 32.0])
}

/// An inset content area, not a second independent floating window.
pub fn inset(ui: &Ui, id: &str, height: f32, body: impl FnOnce()) {
    let _bg = ui.push_style_color(StyleColor::ChildBg, ui.style_color(StyleColor::FrameBg));
    let _pad = ui.push_style_var(StyleVar::WindowPadding([12.0, 10.0]));
    ui.child_window(id)
        .size([0.0, height])
        .flags(imgui::WindowFlags::ALWAYS_USE_WINDOW_PADDING)
        .build(body);
}

/// Keep the label readable independently of the checkbox's native square size.
pub fn setting_toggle(ui: &Ui, id: &str, label: &str, detail: &str, value: &mut bool) -> bool {
    let _id = ui.push_id(id);
    let start = ui.cursor_pos();
    let width = ui.content_region_avail()[0];
    let text_width = (width - 44.0).max(40.0);
    ui.group(|| {
        let _wrap = ui.push_text_wrap_pos_with_pos(start[0] + text_width);
        ui.text_wrapped(label);
        let _font = ui.push_font(ui.fonts().fonts()[2]);
        description(ui, detail);
    });
    let bottom = ui.cursor_pos()[1].max(start[1] + 40.0);
    ui.set_cursor_pos([start[0] + width - 30.0, start[1]]);
    let changed = ui.checkbox("##value", value);
    ui.set_cursor_pos([start[0], bottom + 10.0]);
    ui.separator();
    ui.dummy([0.0, 6.0]);
    changed
}

pub fn key_value(ui: &Ui, key: &str, value: &str) {
    let start = ui.cursor_pos();
    let width = ui.content_region_avail()[0];
    ui.text_disabled(key);
    ui.same_line_with_pos(start[0] + (width - ui.calc_text_size(value)[0]).max(width * 0.5));
    ui.text(value);
}

/// Confirmation is local to its owner, so switching panels cannot execute a stale action.
pub fn confirm_button(ui: &Ui, label: &str, question: &str) -> bool {
    let _id = ui.push_id(label);
    if ui.button(label) {
        ui.open_popup("##confirm");
    }
    let mut confirmed = false;
    // SAFETY: public placement API on the UI thread. Constrain the question's
    // wrap width even when a confirmation contains a long server/file name.
    unsafe {
        imgui::sys::igSetNextWindowSize(
            [(ui.io().display_size[0] - 32.0).min(320.0), 0.0].into(),
            imgui::Condition::Always as i32,
        );
    }
    if let Some(_popup) = ui.begin_popup("##confirm") {
        ui.text_wrapped(question);
        ui.dummy([0.0, 6.0]);
        if primary_button(ui, "Confirm", 100.0) {
            confirmed = true;
            ui.close_current_popup();
        }
        ui.same_line();
        if ui.button_with_size("Cancel", [80.0, 32.0]) {
            ui.close_current_popup();
        }
    }
    confirmed
}

pub fn sheet_header(ui: &Ui, icons: &IconAtlas, title: &str, open: &mut bool) {
    let available = ui.content_region_avail()[0];
    let start = ui.cursor_pos();
    {
        let _font = ui.push_font(ui.fonts().fonts()[3]);
        ui.text(title);
    }
    ui.set_cursor_pos([start[0] + available - 30.0, start[1] - 4.0]);
    if icon_button(ui, icons, "close-sheet", super::icon::CLOSE, "Close") {
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
    icons: &IconAtlas,
    id: &str,
    glyph: Icon,
    label: &str,
    shortcut: &str,
    checked: bool,
) -> bool {
    let width = ui.content_region_avail()[0];
    let _id = ui.push_id(id);
    let clicked = ui.selectable_config("##menu").size([width, 32.0]).build();
    let p = ui.item_rect_min();
    let bottom = ui.item_rect_max()[1];
    icons.draw(ui, glyph, p, [p[0] + 28.0, bottom]);
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
