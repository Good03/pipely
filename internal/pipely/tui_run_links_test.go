package pipely

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func runUIForLinkTest(t *testing.T, u *terminalUI, loaded func() bool) {
	t.Helper()
	ready := make(chan struct{})
	var once sync.Once
	u.app.SetAfterDrawFunc(func(tcell.Screen) {
		if loaded() {
			once.Do(func() { close(ready) })
		}
	})
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
		t.Fatal("TUI did not load")
	}
}

func TestRunBrowserLinks(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		for _, name := range []string{"runs", "active-runs", "group-results", "timeline", "logs"} {
			t.Run(fmt.Sprintf("%s/%dx%d", name, size[0], size[1]), func(t *testing.T) {
				u, screen := simulationUI(t)
				screen.SetSize(size[0], size[1])
				u.client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.HasSuffix(r.URL.Path, "/builds"):
						fmt.Fprint(w, `{"value":[{"id":123,"status":"completed","sourceBranch":"refs/heads/master"},{"id":456,"status":"inProgress"}]}`)
					case strings.HasSuffix(r.URL.Path, "/builds/456"):
						fmt.Fprint(w, `{"id":456,"status":"inProgress"}`)
					case strings.HasSuffix(r.URL.Path, "/timeline"):
						fmt.Fprint(w, `{"records":[]}`)
					case strings.Contains(r.URL.Path, "/logs/"):
						fmt.Fprint(w, "log output")
					default:
						fmt.Fprint(w, `{"value":[]}`)
					}
				})
				opened := make(chan string, 2)
				u.openURL = func(_ context.Context, target string) error { opened <- target; return nil }
				u.build = Build{ID: 456}
				u.pipeline = Pipeline{Id: 7, Name: "Orders"}
				u.home()
				loaded := func() bool { return true }
				switch name {
				case "runs":
					u.runsScreen()
					loaded = func() bool { return strings.Contains(u.footer.GetText(true), "Latest five") }
				case "active-runs":
					u.show("active-runs", "Active runs", table())
					u.renderActiveRuns([]Build{{ID: 123, SourceBranch: "refs/heads/master"}, {ID: 456}})
				case "group-results":
					u.groupResults(Group{Name: "Orders"}, []GroupResult{{RunID: 123}, {RunID: 456}, {Error: "Rejected"}}, nil)
				case "timeline":
					u.timelineScreen()
					loaded = func() bool { return strings.Contains(u.footer.GetText(true), "Run 456:") }
				case "logs":
					u.logID = 1
					u.logScreen()
					loaded = func() bool { return strings.Contains(u.footer.GetText(true), "Following log") }
				}
				runUIForLinkTest(t, u, loaded)
				want := u.client.RunURL(456)
				u.app.QueueUpdateDraw(func() {
					if table, ok := u.content.(*tview.Table); ok && name != "timeline" {
						sendUI(u, tcell.KeyDown, 0)
						row, _ := table.GetSelection()
						if row != 2 {
							t.Errorf("selected row = %d", row)
						}
					}
				})
				u.app.QueueUpdateDraw(func() {
					found := false
					for y := 0; y < size[1]; y++ {
						for x := 0; x < size[0]; x++ {
							_, _, style, _ := screen.GetContent(x, y)
							if style.Url("") != style && style.Url(want) == style {
								found = true
							}
						}
					}
					if !found {
						t.Error("run hyperlink not visible")
					}
					sendUI(u, tcell.KeyRune, 'b')
				})
				select {
				case target := <-opened:
					if target != want {
						t.Fatalf("opened %q, want %q", target, want)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("browser shortcut did not open run")
				}
				u.app.QueueUpdateDraw(func() {
					if u.screen != name {
						t.Errorf("browser action navigated to %s", u.screen)
					}
					for y := 0; y < size[1]; y++ {
						for x := 0; x < size[0]; x++ {
							_, _, style, _ := screen.GetContent(x, y)
							if style.Url("") != style && style.Url(want) == style {
								u.pages.MouseHandler()(tview.MouseLeftClick, tcell.NewEventMouse(x, y, tcell.ButtonNone, tcell.ModNone), func(p tview.Primitive) { u.app.SetFocus(p) })
								return
							}
						}
					}
				})
				select {
				case target := <-opened:
					if target != want {
						t.Fatalf("click opened %q, want %q", target, want)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("click did not open run")
				}
				u.app.QueueUpdateDraw(func() {
					if u.screen != name {
						t.Errorf("click navigated to %s", u.screen)
					}
				})
			})
		}
	}
}

func TestPipelineYAMLShortcut(t *testing.T) {
	u, _ := simulationUI(t)
	const yaml = "trigger: [main]\nsteps:\n- script: echo '[red]hello'\n"
	u.client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pipelines/7"):
			fmt.Fprint(w, `{"configuration":{"path":"pipelines/build.yml","repository":{"id":"repo","type":"azureReposGit"}}}`)
		case strings.HasSuffix(r.URL.Path, "/refs"):
			fmt.Fprint(w, `{"value":[{"name":"refs/heads/main"}]}`)
		case strings.HasSuffix(r.URL.Path, "/items"):
			if r.URL.Query().Get("path") != "/pipelines/build.yml" || r.URL.Query().Get("versionDescriptor.version") != "main" {
				t.Error("wrong YAML source")
			}
			fmt.Fprintf(w, `{"content":%q}`, yaml)
		default:
			fmt.Fprint(w, `{"defaultBranch":"refs/heads/main"}`)
		}
	})
	u.pipelines = []Pipeline{{Id: 7, Name: "Orders"}}
	u.home()
	sendUI(u, tcell.KeyRune, 'y')
	if u.screen != "yaml" {
		t.Fatalf("screen = %s", u.screen)
	}
	runUIForLinkTest(t, u, func() bool { return strings.Contains(u.footer.GetText(true), "Branch: main") })
	u.app.QueueUpdateDraw(func() {
		view, ok := u.content.(*tview.TextView)
		if !ok || view.GetText(true) != yaml {
			t.Error("YAML source was altered")
		}
		sendUI(u, tcell.KeyPgDn, 0)
		sendUI(u, tcell.KeyEscape, 0)
		if u.screen != "pipelines" {
			t.Error("Escape did not return to pipelines")
		}
	})
}
