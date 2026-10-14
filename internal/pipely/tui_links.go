package pipely

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var logURLPattern = regexp.MustCompile(`(?i)https?://[^\s<>"'\x00-\x1f\x7f]+`)

// addRunLinks shares the browser action between run tables while preserving
// their Enter action and other keyboard controls.
func (u *terminalUI) addRunLinks(t *tview.Table, column int, ids []int) {
	t.SetCell(0, column, cell("ADO").SetTextColor(purple).SetSelectable(false))
	for i, id := range ids {
		if id <= 0 || u.client == nil {
			t.SetCell(i+1, column, cell(""))
			continue
		}
		target := u.client.RunURL(id)
		link := cell("").SetText("[::u:" + target + "]Open[::U:-]").SetExpansion(0)
		link.Clicked = func() bool { u.openLink(target); return true }
		t.SetCell(i+1, column, link)
	}
	capture := t.GetInputCapture()
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyRune && e.Rune() == 'b' {
			row, _ := t.GetSelection()
			if row > 0 && row <= len(ids) {
				u.openRun(ids[row-1])
			}
			return nil
		}
		if capture != nil {
			return capture(e)
		}
		return e
	})
}

func (u *terminalUI) openRun(id int) {
	if u.client != nil && id > 0 {
		u.openLink(u.client.RunURL(id))
	}
}

func trimLogURL(s string) string {
	s = strings.TrimRight(s, ".,;!")
	for len(s) > 0 {
		last := s[len(s)-1]
		open := byte(0)
		switch last {
		case ')':
			open = '('
		case ']':
			open = '['
		case '}':
			open = '{'
		default:
			return s
		}
		if strings.Count(s, string(last)) <= strings.Count(s, string(open)) {
			return s
		}
		s = s[:len(s)-1]
	}
	return s
}
func validateLink(target string) error {
	if strings.IndexFunc(target, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return fmt.Errorf("invalid link")
	}
	parsed, e := url.Parse(target)
	if e != nil || parsed.Host == "" || (!strings.EqualFold(parsed.Scheme, "https") && !strings.EqualFold(parsed.Scheme, "http")) {
		return fmt.Errorf("only HTTP and HTTPS links can be opened")
	}
	return nil
}
func logLinks(text string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, raw := range logURLPattern.FindAllString(text, -1) {
		target := trimLogURL(raw)
		if validateLink(target) != nil || seen[target] {
			continue
		}
		seen[target] = true
		result = append(result, target)
	}
	return result
}

// tview forwards these hyperlink styles to terminals as native OSC 8 links.
// Escape all log text first; only validated URLs become style tags.
func hyperlinkText(text string) string {
	var out strings.Builder
	offset := 0
	for _, match := range logURLPattern.FindAllStringIndex(text, -1) {
		target := trimLogURL(text[match[0]:match[1]])
		if validateLink(target) != nil {
			continue
		}
		out.WriteString(tview.Escape(text[offset:match[0]]))
		parsed, _ := url.Parse(target)
		href := parsed.String()
		// The style-tag grammar cannot represent brackets. Encode them in paths
		// and queries; preserve IPv6 host literals by leaving those links to the picker.
		if strings.ContainsAny(parsed.Host, "[]") {
			out.WriteString(tview.Escape(target))
		} else {
			var encoded strings.Builder
			for _, b := range []byte(href) {
				if b >= 128 || b == '[' || b == ']' {
					fmt.Fprintf(&encoded, "%%%02X", b)
				} else {
					encoded.WriteByte(b)
				}
			}
			out.WriteString("[::u:" + encoded.String() + "]" + tview.Escape(target) + "[::U:-]")
		}
		offset = match[0] + len(target)
	}
	out.WriteString(tview.Escape(text[offset:]))
	return out.String()
}
func openBrowserLink(ctx context.Context, target string) error {
	if e := validateLink(target); e != nil {
		return e
	}
	var args []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"open", target}
	case "windows":
		args = []string{"rundll32.exe", "url.dll,FileProtocolHandler", target}
	default:
		args = []string{"xdg-open", target}
	}
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e := exec.CommandContext(deadline, args[0], args[1:]...).Run(); e != nil {
		return fmt.Errorf("could not open the browser: %w", e)
	}
	return nil
}
func (u *terminalUI) openLink(target string) {
	if e := validateLink(target); e != nil {
		u.reportError(e)
		return
	}
	opener := u.openURL
	u.status("Opening link in your browser…")
	go func() {
		e := opener(u.ctx, target)
		if u.ctx.Err() != nil {
			return
		}
		u.app.QueueUpdateDraw(func() {
			if e != nil {
				u.reportError(e)
			} else {
				u.status("Opened link in your browser")
			}
		})
	}()
}
func (u *terminalUI) linkPicker(text string) {
	links := logLinks(text)
	if len(links) == 0 {
		u.status("No HTTP or HTTPS links in this text")
		return
	}
	focus := u.app.GetFocus()
	u.overlayFocus["links"] = focus
	list := tview.NewList().SetSelectedFocusOnly(true).ShowSecondaryText(false)
	for _, target := range links {
		target := target
		list.AddItem(tview.Escape(target), "", 0, func() {
			u.pages.RemovePage("links")
			delete(u.overlayFocus, "links")
			u.app.SetFocus(focus)
			u.openLink(target)
		})
	}
	list.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			u.pages.RemovePage("links")
			u.app.SetFocus(focus)
			return nil
		}
		return e
	})
	u.pages.AddPage("links", newCenteredDialog(list, "Links"), true, true)
	u.app.SetFocus(list)
}
