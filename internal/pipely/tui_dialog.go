package pipely

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var dialogBackground = tcell.NewHexColor(0x30263e)

func dialogButtonStyle() tcell.Style {
	return tcell.StyleDefault.Background(dialogBackground).Foreground(tview.Styles.PrimaryTextColor)
}

func dialogSelectedStyle() tcell.Style {
	return tcell.StyleDefault.Background(purple).Foreground(charcoal)
}

func styleDialogTable(table *tview.Table) {
	for row := 0; row < table.GetRowCount(); row++ {
		for col := 0; col < table.GetColumnCount(); col++ {
			table.GetCell(row, col).SetBackgroundColor(dialogBackground)
		}
	}
	if table.HasFocus() {
		table.SetSelectedStyle(dialogSelectedStyle())
	} else {
		table.SetSelectedStyle(dialogButtonStyle())
	}
}

// Configuration and option pickers share sizing, styling, and input forwarding.
// Their underlying widgets retain focus and scroll to the selected item.
type centeredDialog struct {
	*tview.Box
	widget tview.Primitive
}

type dialogActions struct {
	*tview.Flex
	table   *tview.Table
	content tview.Primitive
	actions *tview.Form
	header  *tview.DropDown
}

func newDialogActions(content tview.Primitive, actions *tview.Form) *dialogActions {
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(content, 0, 1, true).AddItem(actions, 2, 0, false)
	// Flex normally leaves padding transparent; dialogs need a solid surface.
	layout.Box = tview.NewBox()
	table, _ := content.(*tview.Table)
	return &dialogActions{Flex: layout, table: table, content: content, actions: actions}
}

func newCenteredDialog(widget tview.Primitive, title string) *centeredDialog {
	background := dialogBackground
	plain := tcell.StyleDefault.Background(charcoal).Foreground(tview.Styles.PrimaryTextColor)
	selected := dialogSelectedStyle()
	switch w := widget.(type) {
	case *dialogActions:
		w.SetBorder(true).SetTitle(" " + title + " ").SetBorderColor(purple)
		w.SetBackgroundColor(background)
		w.SetBorderPadding(1, 1, 2, 2)
		switch content := w.content.(type) {
		case *tview.List:
			content.SetBorder(false).SetBorderPadding(0, 0, 0, 0)
			content.SetBackgroundColor(background)
			content.SetMainTextColor(tview.Styles.PrimaryTextColor).SetSecondaryTextColor(tview.Styles.SecondaryTextColor)
			content.SetSelectedStyle(selected).SetHighlightFullLine(true)
		case *tview.Table:
			content.SetBorder(false).SetBorderPadding(0, 0, 0, 0)
			content.SetBackgroundColor(background)
		case *tview.TextView:
			content.SetBorder(false).SetBorderPadding(0, 0, 0, 0)
			content.SetBackgroundColor(background)
			content.SetTextColor(tview.Styles.PrimaryTextColor)
		}
		if w.header != nil {
			w.header.SetBackgroundColor(background)
			w.header.SetLabelColor(tview.Styles.SecondaryTextColor)
			w.header.SetFieldStyle(plain).SetFocusedStyle(selected).SetListStyles(plain, selected).SetPrefixStyle(plain)
		}
		w.actions.SetBackgroundColor(background)
		w.actions.SetBorderPadding(1, 0, 0, 0)
		w.actions.SetButtonStyle(dialogButtonStyle()).SetButtonActivatedStyle(selected).SetButtonsAlign(tview.AlignCenter)
	case *tview.Form:
		for i := 0; i < w.GetFormItemCount(); i++ {
			switch field := w.GetFormItem(i).(type) {
			case *tview.DropDown:
				field.SetListStyles(plain, selected).SetFocusedStyle(selected).SetPrefixStyle(plain)
			case *tview.Checkbox:
				field.SetActivatedStyle(selected)
			}
			if notice, ok := w.GetFormItem(i).(*tview.TextView); ok {
				// TextView.Focus calls its finished handler while holding its lock
				// when not scrollable. That can recursively re-enter form focus.
				notice.SetScrollable(true)
			}
		}
		if index, ok := nextFormControl(w, -1, 1); ok {
			w.SetFocus(index)
		}
		w.SetBorder(true).SetTitle(" " + title + " ").SetBorderColor(purple)
		w.SetBackgroundColor(background)
		w.SetLabelColor(tview.Styles.SecondaryTextColor)
		w.SetFieldStyle(plain).SetButtonStyle(dialogButtonStyle()).SetButtonActivatedStyle(selected)
		w.SetBorderPadding(1, 1, 2, 2)
		w.SetButtonsAlign(tview.AlignCenter)
	case *tview.List:
		w.SetMainTextColor(tview.Styles.PrimaryTextColor).SetSecondaryTextColor(tview.Styles.SecondaryTextColor)
		w.SetSelectedStyle(selected).SetHighlightFullLine(true)
		w.SetBorder(true).SetTitle(" " + title + " ").SetBorderColor(purple)
		w.SetBackgroundColor(background)
		w.SetBorderPadding(1, 1, 2, 2)
	case *tview.TextView:
		w.SetBorder(true).SetTitle(" " + title + " ").SetBorderColor(purple)
		w.SetBackgroundColor(background)
		w.SetTextColor(tview.Styles.PrimaryTextColor)
		w.SetBorderPadding(1, 1, 2, 2)
	case *tview.Table:
		w.SetBorder(true).SetTitle(" " + title + " ").SetBorderColor(purple)
		w.SetBackgroundColor(background)
		w.SetBorderPadding(1, 1, 2, 2)
	}
	return &centeredDialog{Box: tview.NewBox(), widget: widget}
}

// Skip informational text and disabled buttons in either direction. The bounded
// search also handles forms that temporarily have no interactive controls.
func nextFormControl(form *tview.Form, current, delta int) (int, bool) {
	items := form.GetFormItemCount()
	total := items + form.GetButtonCount()
	for attempts := 0; attempts < total; attempts++ {
		current = (current + delta + total) % total
		if current < items {
			if _, informational := form.GetFormItem(current).(*tview.TextView); informational {
				continue
			}
		} else if form.GetButton(current - items).IsDisabled() {
			continue
		}
		return current, true
	}
	return 0, false
}

func (d *centeredDialog) size() (width, height int) {
	width, height = 64, 6
	switch w := d.widget.(type) {
	case *dialogActions:
		content := centeredDialog{widget: w.content}
		width, height = content.size()
		height += 3
		if w.header != nil {
			height += 2
		}
	case *tview.Form:
		labelWidth, fieldWidth := 0, 0
		for i := 0; i < w.GetFormItemCount(); i++ {
			item := w.GetFormItem(i)
			labelWidth = max(labelWidth, tview.TaggedStringWidth(item.GetLabel()))
			fieldWidth = max(fieldWidth, item.GetFieldWidth())
			height += max(1, item.GetFieldHeight()) + 1
		}
		buttonsWidth := 0
		for i := 0; i < w.GetButtonCount(); i++ {
			buttonsWidth += tview.TaggedStringWidth(w.GetButton(i).GetLabel()) + 5
		}
		width = max(width, labelWidth+fieldWidth+7, buttonsWidth+6)
	case *tview.List:
		height = 4
		for i := 0; i < w.GetItemCount(); i++ {
			main, secondary := w.GetItemText(i)
			width = max(width, tview.TaggedStringWidth(main)+8, tview.TaggedStringWidth(secondary)+8)
			height++
			if secondary != "" {
				height++
			}
		}
	case *tview.TextView:
		text := w.GetText(true)
		for _, line := range strings.Split(text, "\n") {
			width = max(width, tview.TaggedStringWidth(line)+6)
		}
		height = len(tview.WordWrap(text, min(width, 104)-6)) + 4
	case *tview.Table:
		height = w.GetRowCount() + 4
		columnsWidth := 6
		for col := 0; col < w.GetColumnCount(); col++ {
			columnWidth := 0
			for row := 0; row < w.GetRowCount(); row++ {
				columnWidth = max(columnWidth, tview.TaggedStringWidth(w.GetCell(row, col).Text))
			}
			columnsWidth += columnWidth + 1
		}
		width = max(width, columnsWidth)
	}
	return
}

func (d *centeredDialog) Draw(screen tcell.Screen) {
	d.Box.Draw(screen)
	x, y, availableWidth, availableHeight := d.GetInnerRect()
	width, height := d.size()
	width = min(width, 104, max(0, availableWidth-4))
	switch widget := d.widget.(type) {
	case *tview.TextView:
		height = len(tview.WordWrap(widget.GetText(true), max(1, width-6))) + 4
	case *dialogActions:
		if view, ok := widget.content.(*tview.TextView); ok {
			height = len(tview.WordWrap(view.GetText(true), max(1, width-6))) + 7
		}
	}
	height = min(height, max(0, availableHeight-2))
	d.widget.SetRect(x+(availableWidth-width)/2, y+(availableHeight-height)/2, width, height)
	switch widget := d.widget.(type) {
	case *dialogActions:
		if widget.table != nil {
			styleDialogTable(widget.table)
		}
	case *tview.Table:
		styleDialogTable(widget)
	}
	d.widget.Draw(screen)
}

func (d *centeredDialog) Focus(delegate func(tview.Primitive)) { delegate(d.widget) }
func (d *centeredDialog) HasFocus() bool                       { return d.widget.HasFocus() }
func (d *centeredDialog) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return d.WrapInputHandler(d.widget.InputHandler())
}
func (d *centeredDialog) PasteHandler() func(string, func(tview.Primitive)) {
	return d.WrapPasteHandler(d.widget.PasteHandler())
}
func (d *centeredDialog) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return d.widget.MouseHandler()
}

func isChoiceScreen(name string) bool {
	switch name {
	case "contexts", "projects", "stages", "group-detail":
		return true
	}
	return false
}

// Message dialogs use the same frame, padding, and action row as other pickers.
func (u *terminalUI) messageDialog(page, title, text string, labels []string, selected func(int)) {
	focus := u.app.GetFocus()
	u.overlayFocus[page] = focus
	view := tview.NewTextView().SetText(text).SetWrap(true).SetWordWrap(true).SetScrollable(true)
	actions := tview.NewForm()
	for i, label := range labels {
		index := i
		actions.AddButton(label, func() {
			u.closeOverlay(page)
			if selected != nil {
				selected(index)
			}
		})
	}
	actions.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyLeft:
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		case tcell.KeyRight:
			return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
		case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
			view.InputHandler()(event, func(p tview.Primitive) { u.app.SetFocus(p) })
			return nil
		}
		return event
	})
	panel := newCenteredDialog(newDialogActions(view, actions), title)
	u.pages.AddPage(page, panel, true, true)
	u.app.SetFocus(actions)
}

func (u *terminalUI) closeOverlay(page string) {
	focus := u.overlayFocus[page]
	delete(u.overlayFocus, page)
	u.pages.RemovePage(page)
	if focus != nil {
		u.app.SetFocus(focus)
	}
}
