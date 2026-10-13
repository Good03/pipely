package pipely

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestLogLinkExtraction(t *testing.T) {
	text := `Read (https://example.com/docs). Then https://example.com/a_(b) and https://example.com/docs. Ignore javascript:alert(1).`
	want := []string{"https://example.com/docs", "https://example.com/a_(b)"}
	if got := logLinks(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	for _, target := range []string{"file:///tmp/test", "javascript:alert(1)", "https://example.com/\ncommand", "https://"} {
		if validateLink(target) == nil {
			t.Errorf("accepted %q", target)
		}
	}
}
func TestNativeLogHyperlinksPreservePlainText(t *testing.T) {
	u, screen := simulationUI(t)
	text := "See (https://example.com/docs?a=1&b=2). [ordinary text]\n"
	view := tview.NewTextView().SetDynamicColors(true).SetText(styledLog(text))
	u.show("logs", "Logs", view)
	u.app.ForceDraw()
	if got := view.GetText(true); got != text {
		t.Fatalf("copy text changed: %q", got)
	}
	width, height := screen.Size()
	var linked strings.Builder
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, _, style, _ := screen.GetContent(x, y)
			if style.Url("") != style {
				linked.WriteRune(r)
			}
		}
	}
	if linked.String() != "https://example.com/docs?a=1&b=2" {
		t.Fatalf("native link span = %q", linked.String())
	}
}
func TestLinkMarkupCannotInjectStyles(t *testing.T) {
	text := "https://example.com/path?q=[red] normal [green] text"
	view := tview.NewTextView().SetDynamicColors(true).SetText(hyperlinkText(text))
	if got := view.GetText(true); got != text {
		t.Fatalf("markup changed original text: %q", got)
	}
	if !strings.Contains(hyperlinkText(text), "%5Bred%5D") {
		t.Fatal("URL target was not safely encoded")
	}
}
func TestDiagnosticLinkPickerOpensOnlySelectedURL(t *testing.T) {
	u, _ := simulationUI(t)
	opened := make(chan string, 1)
	u.openURL = func(_ context.Context, target string) error { opened <- target; return nil }
	u.home()
	u.info("Failure details", "See https://example.com/fix and https://example.com/other")
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
	u.app.QueueUpdateDraw(func() { sendUI(u, tcell.KeyRune, 'o') })
	select {
	case <-opened:
		t.Fatal("rendering picker opened a browser automatically")
	default:
	}
	u.app.QueueUpdateDraw(func() {
		list, ok := u.app.GetFocus().(*tview.List)
		if !ok {
			t.Error("link picker not focused")
			return
		}
		list.SetCurrentItem(1)
		sendUI(u, tcell.KeyEnter, 0)
	})
	select {
	case target := <-opened:
		if target != "https://example.com/other" {
			t.Fatal(target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("selected link did not open")
	}
	u.app.QueueUpdateDraw(func() {
		front, _ := u.pages.GetFrontPage()
		if front != "info" {
			t.Error("picker did not restore diagnostics")
		}
	})
}
