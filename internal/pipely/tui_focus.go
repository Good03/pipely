package pipely

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Preserve the cursor position on blur without painting it as keyboard focus.
func focusTableSelection(t *tview.Table, inactive tcell.Style) {
	active := tcell.StyleDefault.Background(purple).Foreground(charcoal)
	t.SetSelectedStyle(inactive)
	t.SetFocusFunc(func() { t.SetSelectedStyle(active) })
	t.SetBlurFunc(func() { t.SetSelectedStyle(inactive) })
}
