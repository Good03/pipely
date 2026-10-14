package pipely

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestFooterNoticeLayoutAndLink(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			u, screen := simulationUI(t)
			screen.SetSize(size[0], size[1])
			u.home()
			for _, status := range []string{"Ready", strings.Repeat("Long status message ", 12)} {
				u.status(status)
				u.app.ForceDraw()
				var notice, linked strings.Builder
				x, y, width, height := u.footerNotice.GetRect()
				if x+width != size[0] || y+height != size[1] {
					t.Fatal("notice is not at the bottom-right corner")
				}
				for row := y; row < y+height; row++ {
					var line strings.Builder
					for col := x; col < x+width; col++ {
						r, _, style, _ := screen.GetContent(col, row)
						line.WriteRune(r)
						if style.Url("") != style {
							if style.Url("https://github.com/Good03/pipely/issues") != style {
								t.Fatal("incorrect issue link target")
							}
							linked.WriteRune(r)
						}
					}
					text := strings.TrimSpace(line.String())
					if text != "" {
						if !strings.HasSuffix(line.String(), text) {
							t.Fatal("notice text is not right aligned")
						}
						if notice.Len() > 0 {
							notice.WriteByte(' ')
						}
						notice.WriteString(text)
					}
				}
				if notice.String() != "Early version of Pipely. Report bugs on GitHub." || linked.String() != "GitHub" {
					t.Fatalf("notice = %q, link = %q", notice.String(), linked.String())
				}
			}
			// Exercise navigation through the root layout with the new footer row.
			sendUI(u, tcell.KeyF4, 0)
			sendUI(u, tcell.KeyDown, 0)
			if u.app.GetFocus() == u.tabs || u.footerNotice.HasFocus() {
				t.Fatal("footer interfered with returning to content")
			}
		})
	}
}

func TestFooterIssueClick(t *testing.T) {
	u, screen := simulationUI(t)
	u.home()
	opened := make(chan string, 2)
	u.openURL = func(_ context.Context, target string) error {
		opened <- target
		return nil
	}
	ready := make(chan struct{})
	var once sync.Once
	u.app.SetAfterDrawFunc(func(tcell.Screen) { once.Do(func() { close(ready) }) })
	done := make(chan error, 1)
	go func() { done <- u.app.Run() }()
	t.Cleanup(func() {
		u.cancel()
		u.app.Stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("TUI did not stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("TUI did not render")
	}
	u.app.QueueUpdateDraw(func() {
		if !u.mouseEnabled {
			t.Error("mouse capture must start enabled for footer clicks")
		}
		focus := u.app.GetFocus()
		width, height := screen.Size()
		handler := u.pages.MouseHandler()
		setFocus := func(p tview.Primitive) { u.app.SetFocus(p) }
		// Click ordinary notice text, then the GitHub link through the root.
		handler(tview.MouseLeftClick, tcell.NewEventMouse(width-10, height-2, tcell.ButtonNone, tcell.ModNone), setFocus)
		select {
		case <-opened:
			t.Error("plain notice text opened a browser")
		default:
		}
		// Let Application derive the click from actual press/release events.
		u.app.QueueEvent(tcell.NewEventMouse(width-7, height-1, tcell.Button1, tcell.ModNone))
		u.app.QueueEvent(tcell.NewEventMouse(width-7, height-1, tcell.ButtonNone, tcell.ModNone))
		if u.app.GetFocus() != focus {
			t.Error("notice click took keyboard focus")
		}
		sendUI(u, tcell.KeyF3, 0)
		if u.mouseEnabled {
			t.Error("F3 did not disable mouse capture")
		}
		sendUI(u, tcell.KeyF3, 0)
	})
	select {
	case target := <-opened:
		if target != githubIssuesURL {
			t.Fatal(target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("footer click did not open GitHub issues")
	}
}
