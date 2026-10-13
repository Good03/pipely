package pipely

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *terminalUI) dropdownOpen() bool {
	if u.form == nil {
		return false
	}
	for i := 0; i < u.form.GetFormItemCount(); i++ {
		if d, ok := u.form.GetFormItem(i).(*tview.DropDown); ok && d.IsOpen() {
			return true
		}
	}
	return false
}
func (u *terminalUI) input(e *tcell.EventKey) *tcell.EventKey {
	if e.Key() == tcell.KeyCtrlC {
		u.cancel()
		u.app.Stop()
		return nil
	}
	if e.Key() == tcell.KeyF1 {
		u.help()
		return nil
	}
	if e.Key() == tcell.KeyF2 {
		u.copyCurrent(false)
		return nil
	}
	if e.Key() == tcell.KeyCtrlY {
		u.copyCurrent(true)
		return nil
	}
	if e.Key() == tcell.KeyF3 {
		u.mouseEnabled = !u.mouseEnabled
		u.app.EnableMouse(u.mouseEnabled)
		u.status(fmt.Sprintf("Mouse capture: %t · disable to select text with your terminal mouse", u.mouseEnabled))
		return nil
	}
	front, overlay := u.pages.GetFrontPage()
	if front == "main" {
		if e.Key() == tcell.KeyRune && e.Rune() == ':' && !u.dropdownOpen() {
			switch u.app.GetFocus().(type) {
			case *tview.InputField, *tview.TextArea:
			default:
				u.commandBar.SetText("")
				u.app.SetFocus(u.commandBar)
				return nil
			}
		}
		if u.app.GetFocus() == u.tabs {
			return e
		}
		if e.Key() == tcell.KeyF4 {
			u.app.SetFocus(u.tabs)
			return nil
		}
		if e.Key() == tcell.KeyUp && u.form == nil {
			if t, ok := u.app.GetFocus().(*tview.Table); ok {
				row, _ := t.GetSelection()
				if row <= 1 {
					u.app.SetFocus(u.tabs)
					return nil
				}
			}
		}
	}
	if front != "main" {
		if _, ok := overlay.(*tview.Modal); ok {
			switch e.Key() {
			case tcell.KeyLeft, tcell.KeyUp:
				return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
			case tcell.KeyRight, tcell.KeyDown:
				return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
			}
		}
	}
	if e.Key() == tcell.KeyEscape {
		if front != "main" {
			u.pages.RemovePage(front)
			if f := u.overlayFocus[front]; f != nil {
				u.app.SetFocus(f)
			}
			delete(u.overlayFocus, front)
			return nil
		}
		if u.dropdownOpen() {
			return e
		}
		if u.back != nil {
			u.back()
			return nil
		}
	}
	if front == "main" && u.form != nil && !u.dropdownOpen() {
		item, button := u.form.GetFocusedItemIndex()
		index := item
		if button >= 0 {
			index = u.form.GetFormItemCount() + button
		}
		delta := 0
		switch e.Key() {
		case tcell.KeyUp:
			delta = -1
		case tcell.KeyDown:
			delta = 1
		case tcell.KeyLeft:
			if button >= 0 {
				delta = -1
			}
		case tcell.KeyRight:
			if button >= 0 {
				delta = 1
			}
		}
		total := u.form.GetFormItemCount() + u.form.GetButtonCount()
		if delta != 0 && total > 0 {
			index = (index + delta + total) % total
			u.form.SetFocus(index)
			u.app.SetFocus(u.form)
			return nil
		}
	}
	return e
}
func (u *terminalUI) setBack(name string) {
	u.back = nil
	switch name {
	case "contexts", "run-form", "save-group", "groups":
		u.back = u.home
	case "context-form":
		u.back = u.contextScreen
	case "projects":
		u.back = u.contextForm
	case "auth":
		u.back = func() {
			if u.client != nil {
				u.home()
			} else {
				u.contextForm()
			}
		}
	case "runs", "active-runs":
		u.back = u.home
	case "timeline":
		u.back = func() {
			if u.runOrigin == "active-runs" {
				u.activeRunsScreen()
			} else {
				u.runsScreen()
			}
		}
	case "logs":
		u.back = u.timelineScreen
	case "group-detail", "group-results":
		u.back = u.groupScreen
	case "approvals":
		u.back = func() {
			if u.build.ID > 0 {
				u.timelineScreen()
			} else {
				u.home()
			}
		}
	}
}
func nativeCopy(ctx context.Context, text string) error {
	candidates := [][]string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbcopy"}}
	case "windows":
		candidates = [][]string{{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$input | Set-Clipboard"}}
	default:
		candidates = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, args := range candidates {
		path, e := exec.LookPath(args[0])
		if e != nil {
			continue
		}
		deadline, cancel := context.WithTimeout(ctx, 2*time.Second)
		cmd := exec.CommandContext(deadline, path, args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		e = cmd.Run()
		cancel()
		if e == nil {
			return nil
		}
	}
	return fmt.Errorf("system clipboard is unavailable")
}
func (u *terminalUI) copyCurrent(errorOnly bool) {
	text := ""
	if errorOnly {
		text = u.lastError
	} else {
		switch v := u.app.GetFocus().(type) {
		case *tview.TextView:
			text = v.GetText(true)
		case *tview.Table:
			row, _ := v.GetSelection()
			if row >= 0 && row < v.GetRowCount() {
				parts := []string{}
				for col := 0; col < v.GetColumnCount(); col++ {
					parts = append(parts, tview.Unescape(v.GetCell(row, col).Text))
				}
				text = strings.Join(parts, "  ")
			}
		}
		if text == "" && u.terminalScreen != nil {
			w, h := u.terminalScreen.Size()
			var b strings.Builder
			for y := 0; y < h; y++ {
				var line strings.Builder
				for x := 0; x < w; x++ {
					r, _, _, _ := u.terminalScreen.GetContent(x, y)
					if r == 0 {
						r = ' '
					}
					line.WriteRune(r)
				}
				b.WriteString(strings.TrimRight(line.String(), " "))
				b.WriteByte('\n')
			}
			text = b.String()
		}
	}
	if text == "" {
		u.status("Nothing to copy")
		return
	}
	if u.copyText != nil {
		if e := u.copyText(u.ctx, text); e == nil {
			u.status("Copied to clipboard")
			return
		}
	}
	if u.terminalScreen != nil {
		u.terminalScreen.SetClipboard([]byte(text))
		u.status("Sent to terminal clipboard · mouse selection is also available with F3 mouse capture off")
	} else {
		u.status("Clipboard unavailable; use terminal text selection")
	}
}
