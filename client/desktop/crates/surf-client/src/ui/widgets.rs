//! Surf content and geometry over public ImGui widgets: IDs, navigation and activation
//! remain owned by ImGui, never by a parallel invisible-button interaction system.
use imgui::{StyleColor, StyleVar, Ui};

pub fn icon_button(ui: &Ui, id: &str, glyph: &str, help: &str) -> bool {
    let _id = ui.push_id(id);
    let _align = ui.push_style_var(StyleVar::ButtonTextAlign([0.5, 0.5]));
    let clicked = ui.button_with_size(format!("{glyph}###control"), [30.0, 30.0]);
    if ui.is_item_hovered() {
        ui.tooltip_text(help);
    }
    clicked
}

pub fn section(ui: &Ui, title: &str) {
    ui.dummy([0.0, 8.0]);
    ui.text_disabled(title);
    ui.dummy([0.0, 2.0]);
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
    let draw = ui.get_window_draw_list();
    let color = ui.style_color(StyleColor::Text);
    draw.add_text([p[0] + 6.0, p[1] + 8.0], color, glyph);
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
