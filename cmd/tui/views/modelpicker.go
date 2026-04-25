package views

import "github.com/nickqiaoo/tadk/cmd/tui/internal/render"

func RenderModelPicker(theme render.Theme, width int, items []PickerItem, selected int) string {
	return renderPicker(theme, width, "Models", "", items, selected)
}
