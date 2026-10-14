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

func simulationUI(t *testing.T) (*terminalUI, tcell.SimulationScreen) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	u := newTerminalUI(ctx, cancel, Config{Org: "example", Project: "services", Auth: "az"}, "")
	screen := tcell.NewSimulationScreen("")
	if e := screen.Init(); e != nil {
		t.Fatal(e)
	}
	u.app.SetScreen(screen)
	screen.SetSize(120, 35)
	t.Cleanup(screen.Fini)
	return u, screen
}
func TestTUIStageSelection(t *testing.T) {
	u, screen := simulationUI(t)
	stages, e := ParseStages("stages:\n- stage: Build\n- stage: Test\n- stage: Dev\n- stage: Nonprod\n- stage: Prod\n")
	if e != nil {
		t.Fatal(e)
	}
	u.stageScreen(Pipeline{Id: 1, Name: "orders"}, Plan{Options: RunOptions{PipelineID: 1}, Stages: stages}, nil)
	table := u.app.GetFocus().(*tview.Table)
	table.Select(5, 0)
	capture := table.GetInputCapture()
	capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	for row := 1; row <= 5; row++ {
		if !strings.Contains(tview.Unescape(table.GetCell(row, 0).Text), "[x]") {
			t.Fatalf("stage %d not selected", row)
		}
	}
	table.Select(2, 0)
	capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if !strings.Contains(u.footer.GetText(false), "Required") {
		t.Fatal("dependency could be removed")
	}
	u.app.ForceDraw()
	cells, w, h := screen.GetContents()
	var text strings.Builder
	for _, c := range cells {
		if len(c.Runes) > 0 {
			text.WriteRune(c.Runes[0])
		}
	}
	if w != 120 || h != 35 || !strings.Contains(text.String(), "Prod") {
		t.Fatal("stage screen did not render")
	}
	table.Select(5, 0)
	capture(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	if strings.Contains(tview.Unescape(table.GetCell(5, 0).Text), "[x]") {
		t.Fatal("Prod was not deselected")
	}
}
func TestTUISearchAndNavigation(t *testing.T) {
	u, _ := simulationUI(t)
	u.pipelines = []Pipeline{{Id: 1, Name: "orders"}, {Id: 2, Name: "payments"}}
	u.home()
	table := u.app.GetFocus().(*tview.Table)
	sendUI(u, tcell.KeyRune, '/')
	search := u.app.GetFocus().(*tview.InputField)
	search.SetText("payment")
	if table.GetRowCount() != 2 || table.GetCell(1, 2).Text != "payments" {
		t.Fatal("search did not filter")
	}
	sendUI(u, tcell.KeyEnter, 0)
	sendUI(u, tcell.KeyRune, ' ')
	if !u.selected[2] || u.selected[1] {
		t.Fatal("wrong pipeline selected")
	}
	sendUI(u, tcell.KeyRune, 'o')
	if u.screen != "contexts" {
		t.Fatal("context switch unavailable")
	}
}

func sendUI(u *terminalUI, key tcell.Key, r rune) {
	event := u.input(tcell.NewEventKey(key, r, tcell.ModNone))
	if event == nil {
		return
	}
	// Match Application.Run: route input through the root and its containers.
	// Sending directly to the focused field hides broken container forwarding.
	if handler := u.pages.InputHandler(); handler != nil {
		handler(event, func(p tview.Primitive) { u.app.SetFocus(p) })
	}
}
func TestTypedRunOptionsAndArrowNavigation(t *testing.T) {
	u, _ := simulationUI(t)
	u.renderRunForm(Pipeline{Id: 1, Name: "orders"}, RunOptions{PipelineID: 1}, Metadata{Branch: "main", Branches: []string{"main"}, Parameters: []Parameter{{Name: "tests", Type: "boolean", Default: true}, {Name: "environment", Type: "string", Values: []any{"dev", "prod"}, Default: "dev"}}})
	if _, ok := u.form.GetFormItem(0).(*tview.DropDown); !ok {
		t.Fatal("branch is not a dropdown")
	}
	if _, ok := u.form.GetFormItem(1).(*tview.Checkbox); !ok {
		t.Fatal("boolean is not a checkbox")
	}
	sendUI(u, tcell.KeyDown, 0)
	item, _ := u.form.GetFocusedItemIndex()
	if item != 1 {
		t.Fatal("Down did not move to checkbox")
	}
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyDown, 0)
	_, button := u.form.GetFocusedItemIndex()
	if button != 0 {
		t.Fatal("Down did not reach buttons")
	}
	sendUI(u, tcell.KeyRight, 0)
	_, button = u.form.GetFocusedItemIndex()
	if button != 1 {
		t.Fatal("Right did not move between buttons")
	}
	u.form.SetFocus(0)
	u.app.SetFocus(u.form)
	sendUI(u, tcell.KeyEnter, 0)
	if !u.dropdownOpen() {
		t.Fatal("branch dropdown did not open")
	}
	sendUI(u, tcell.KeyEscape, 0)
	if u.dropdownOpen() || u.screen != "run-form" {
		t.Fatal("Esc should close dropdown first")
	}
	sendUI(u, tcell.KeyEnter, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if u.dropdownOpen() {
		t.Fatal("choosing branch left dropdown open")
	}
	sendUI(u, tcell.KeyEscape, 0)
	if u.screen != "pipelines" {
		t.Fatal("Esc did not leave run configuration")
	}
}
func TestCopyErrorsAndPrettyLogs(t *testing.T) {
	u, _ := simulationUI(t)
	copied := ""
	u.copyText = func(_ context.Context, text string) error { copied = text; return nil }
	u.lastError = "Azure GET source repository: 404 Not Found"
	sendUI(u, tcell.KeyCtrlY, 0)
	if copied != u.lastError {
		t.Fatal("full error was not copied")
	}
	view := tview.NewTextView().SetText("build output\nsecond line\n")
	u.show("logs", "Logs", view)
	sendUI(u, tcell.KeyF2, 0)
	if copied != "build output\nsecond line\n" {
		t.Fatal("log text was not copied")
	}
}
func TestEscCancelsConfirmationAndForms(t *testing.T) {
	u, _ := simulationUI(t)
	u.home()
	confirmed := false
	u.confirm("Queue?", func() { confirmed = true })
	sendUI(u, tcell.KeyEscape, 0)
	front, _ := u.pages.GetFrontPage()
	if confirmed || front != "main" {
		t.Fatal("Esc did not cancel confirmation")
	}
	u.contextScreen()
	sendUI(u, tcell.KeyEscape, 0)
	if u.screen != "pipelines" {
		t.Fatal("Esc did not leave context screen")
	}
}
func TestFooterSeparatesStatusAndHints(t *testing.T) {
	u, _ := simulationUI(t)
	u.home()
	u.status("Ready")
	u.app.ForceDraw()
	if !strings.Contains(u.footer.GetText(false), "STATUS") || strings.Contains(u.footer.GetText(false), "New run") {
		t.Fatal("status contains shortcut clutter")
	}
	if !strings.Contains(u.hints.GetText(false), "New run") || !strings.Contains(u.hints.GetText(false), "Esc") {
		t.Fatal("contextual hints missing")
	}
}

func TestActiveRunTabAndReturn(t *testing.T) {
	u, _ := simulationUI(t)
	u.home()
	sendUI(u, tcell.KeyF4, 0)
	if u.app.GetFocus() != u.tabs {
		t.Fatal("F4 did not focus tabs")
	}
	sendUI(u, tcell.KeyRight, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if u.screen != "active-runs" {
		t.Fatal("active runs tab did not open")
	}
	if u.app.GetFocus() != u.tabs {
		t.Fatal("tab selection moved focus into content")
	}
	u.renderActiveRuns([]Build{{ID: 123, Status: "inProgress", Definition: Pipeline{Id: 4, Name: "orders"}, SourceBranch: "refs/heads/main"}})
	if u.app.GetFocus() != u.tabs {
		t.Fatal("refresh stole tab focus")
	}
	sendUI(u, tcell.KeyDown, 0)
	table := u.app.GetFocus().(*tview.Table)
	if table.GetCell(1, 1).Text != "orders" {
		t.Fatal("pipeline name missing")
	}
	u.runOrigin = "active-runs"
	u.setBack("timeline")
	u.back()
	if u.screen != "active-runs" {
		t.Fatal("timeline did not return to active tab")
	}
}
func TestPrettyLogCopyHasNoStyleTags(t *testing.T) {
	u, _ := simulationUI(t)
	copied := ""
	u.copyText = func(_ context.Context, text string) error { copied = text; return nil }
	view := tview.NewTextView().SetDynamicColors(true).SetText(styledLog("##[error]failed [step]\n"))
	u.show("logs", "Logs", view)
	sendUI(u, tcell.KeyF2, 0)
	if copied != "ERROR  failed [step]\n" {
		t.Fatalf("copied %q", copied)
	}
}

func TestFooterFitsStandardTerminal(t *testing.T) {
	u, screen := simulationUI(t)
	screen.SetSize(80, 24)
	u.home()
	u.status("Ready")
	u.app.ForceDraw()
	contents, width, height := screen.GetContents()
	lines := []string{}
	for y := height - 6; y < height; y++ {
		var line strings.Builder
		for x := 0; x < width; x++ {
			c := contents[y*width+x]
			if len(c.Runes) > 0 {
				line.WriteRune(c.Runes[0])
			}
		}
		lines = append(lines, line.String())
	}
	footer := strings.Join(lines, "\n")
	for _, want := range []string{"STATUS", "Ready", "Esc", "F2", "New run", ": Command"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("missing %s in footer:\n%s", want, footer)
		}
	}
}

func TestEmptyGroupsRemainNavigableAfterDraw(t *testing.T) {
	u, _ := simulationUI(t)
	u.groupScreen()
	u.app.ForceDraw()
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyUp, 0)
}

func TestRapidHeaderSwitchingKeepsFocus(t *testing.T) {
	u, _ := simulationUI(t)
	u.home()
	sendUI(u, tcell.KeyF4, 0)
	for i := 0; i < 100; i++ {
		u.tabs.Select(0, 2)
		sendUI(u, tcell.KeyEnter, 0)
		u.app.ForceDraw()
		if u.screen != "groups" || u.app.GetFocus() != u.tabs {
			t.Fatal("Groups stole header focus")
		}
		sendUI(u, tcell.KeyLeft, 0)
		sendUI(u, tcell.KeyEnter, 0)
		u.app.ForceDraw()
		if u.screen != "active-runs" || u.app.GetFocus() != u.tabs {
			t.Fatal("Active runs stole header focus")
		}
		sendUI(u, tcell.KeyLeft, 0)
		sendUI(u, tcell.KeyEnter, 0)
		u.app.ForceDraw()
		if u.screen != "pipelines" || u.app.GetFocus() != u.tabs {
			t.Fatal("Pipelines stole header focus")
		}
		sendUI(u, tcell.KeyDown, 0)
		if u.app.GetFocus() == u.tabs {
			t.Fatal("Down did not enter content")
		}
		sendUI(u, tcell.KeyDown, 0)
		sendUI(u, tcell.KeyUp, 0)
		if u.app.GetFocus() != u.tabs {
			t.Fatal("Up did not return to header")
		}
	}
}

func TestLeavingActiveTabCancelsRequest(t *testing.T) {
	u, _ := simulationUI(t)
	started, canceled := make(chan struct{}), make(chan struct{})
	u.client = testClient(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) })
	u.home()
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
			t.Error("TUI event loop did not stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("TUI did not draw")
	}
	u.app.QueueUpdateDraw(func() { u.app.SetFocus(u.tabs); u.tabs.Select(0, 1); sendUI(u, tcell.KeyEnter, 0) })
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("active request did not start")
	}
	u.app.QueueUpdateDraw(func() { sendUI(u, tcell.KeyLeft, 0); sendUI(u, tcell.KeyEnter, 0) })
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("obsolete active request was not canceled")
	}
	u.app.QueueUpdateDraw(func() {
		if u.screen != "pipelines" || u.app.GetFocus() != u.tabs {
			t.Error("stale completion changed tab or focus")
		}
	})
}

func purpleSelectionCells(screen tcell.Screen, widget tview.Primitive) int {
	x, y, width, height := widget.GetRect()
	count := 0
	for row := y; row < y+height; row++ {
		for col := x; col < x+width; col++ {
			_, _, style, _ := screen.GetContent(col, row)
			_, background, _ := style.Decompose()
			if background == purple {
				count++
			}
		}
	}
	return count
}
func TestOnlyFocusedAreaHasSelectionHighlight(t *testing.T) {
	u, screen := simulationUI(t)
	u.pipelines = []Pipeline{{Id: 1, Name: "orders"}, {Id: 2, Name: "payments"}}
	u.home()
	content := u.app.GetFocus().(*tview.Table)
	content.Select(2, 0)
	u.app.ForceDraw()
	if purpleSelectionCells(screen, content) == 0 || purpleSelectionCells(screen, u.tabs) != 0 {
		t.Fatal("content focus did not have the only selection highlight")
	}
	sendUI(u, tcell.KeyF4, 0)
	u.app.ForceDraw()
	if purpleSelectionCells(screen, u.tabs) == 0 || purpleSelectionCells(screen, content) != 0 {
		t.Fatal("header focus left a selected content row visible")
	}
	row, _ := content.GetSelection()
	if row != 2 {
		t.Fatal("moving focus discarded the content cursor")
	}
	sendUI(u, tcell.KeyF1, 0)
	u.app.ForceDraw()
	_, overlay := u.pages.GetFrontPage()
	panel := overlay.(*centeredDialog).widget
	ox, oy, ow, oh := panel.GetRect()
	outsideHighlighted := func(widget tview.Primitive) bool {
		x, y, width, height := widget.GetRect()
		for row := y; row < y+height; row++ {
			for col := x; col < x+width; col++ {
				if col >= ox && col < ox+ow && row >= oy && row < oy+oh {
					continue // The overlay's own focused button is allowed here.
				}
				_, _, style, _ := screen.GetContent(col, row)
				_, background, _ := style.Decompose()
				if background == purple {
					return true
				}
			}
		}
		return false
	}
	if u.tabs.HasFocus() || content.HasFocus() || outsideHighlighted(u.tabs) || outsideHighlighted(content) {
		t.Fatal("unfocused areas remained highlighted behind the help dialog")
	}
	sendUI(u, tcell.KeyEscape, 0)
	sendUI(u, tcell.KeyDown, 0)
	u.app.ForceDraw()
	if purpleSelectionCells(screen, content) == 0 || purpleSelectionCells(screen, u.tabs) != 0 {
		t.Fatal("returning to content did not transfer the highlight")
	}
	row, _ = content.GetSelection()
	if row != 2 {
		t.Fatal("returning to content lost its previous cursor")
	}
}
func TestBackgroundRefreshDoesNotHighlightContent(t *testing.T) {
	u, screen := simulationUI(t)
	u.home()
	sendUI(u, tcell.KeyF4, 0)
	u.tabs.Select(0, 1)
	sendUI(u, tcell.KeyEnter, 0)
	u.renderActiveRuns([]Build{{ID: 123, Definition: Pipeline{Id: 4, Name: "orders"}, Status: "inProgress"}})
	u.app.ForceDraw()
	content := u.content.(*tview.Table)
	if purpleSelectionCells(screen, content) != 0 || purpleSelectionCells(screen, u.tabs) == 0 {
		t.Fatal("background refresh introduced a second selection highlight")
	}
	sendUI(u, tcell.KeyDown, 0)
	u.app.ForceDraw()
	if purpleSelectionCells(screen, content) == 0 || purpleSelectionCells(screen, u.tabs) != 0 {
		t.Fatal("Down did not transfer the highlight to refreshed content")
	}
}

func TestMissingYAMLBranchCanRecover(t *testing.T) {
	u, _ := simulationUI(t)
	u.client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("missing YAML must not queue or preview a run")
			w.WriteHeader(400)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/pipelines/"):
			fmt.Fprint(w, `{"configuration":{"path":"azure-pipelines.yml","repository":{"id":"repo","type":"azureReposGit"}}}`)
		case strings.HasSuffix(r.URL.Path, "/refs"):
			fmt.Fprint(w, `{"value":[{"name":"refs/heads/main"},{"name":"refs/heads/release"}]}`)
		case strings.HasSuffix(r.URL.Path, "/items"):
			if r.URL.Query().Get("versionDescriptor.version") == "main" {
				w.Header().Set("X-VSS-E2EID", "missing-file-request")
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, `{"content":"parameters:\n- name: tests\n  type: boolean\n  default: true\nstages:\n- stage: Build\n"}`)
		default:
			fmt.Fprint(w, `{"defaultBranch":"refs/heads/main"}`)
		}
	})
	type snapshot struct {
		branch    string
		canLaunch bool
		errorText string
	}
	rendered := make(chan snapshot, 32)
	u.app.SetAfterDrawFunc(func(tcell.Screen) {
		if u.busy || u.form == nil || u.form.GetFormItemCount() == 0 {
			return
		}
		dropdown, ok := u.form.GetFormItem(0).(*tview.DropDown)
		if !ok {
			return
		}
		_, branch := dropdown.GetCurrentOption()
		s := snapshot{branch, u.form.GetButtonIndex("Select stages") >= 0, u.lastError}
		select {
		case rendered <- s:
		default:
		}
	})
	u.runForm(Pipeline{Id: 7, Name: "orders"}, RunOptions{PipelineID: 7, Branch: "main"})
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
	await := func(branch string, canLaunch bool) {
		t.Helper()
		timeout := time.After(3 * time.Second)
		for {
			select {
			case s := <-rendered:
				if s.branch == branch && s.canLaunch == canLaunch {
					if branch == "main" && !strings.Contains(s.errorText, "was not found on branch main") {
						t.Fatal("missing branch message unavailable")
					}
					return
				}
			case <-timeout:
				t.Fatalf("did not render branch %s with canLaunch=%t", branch, canLaunch)
			}
		}
	}
	await("main", false)
	u.app.QueueUpdateDraw(func() { sendUI(u, tcell.KeyEnter, 0); sendUI(u, tcell.KeyDown, 0); sendUI(u, tcell.KeyEnter, 0) })
	await("release", true)
}

func TestColonFocusesCommandBarWithoutChangingTab(t *testing.T) {
	u, _ := simulationUI(t)
	u.home()
	if u.app.GetFocus() == u.commandBar {
		t.Fatal("command bar should require a shortcut")
	}
	sendUI(u, tcell.KeyF4, 0)
	u.tabs.Select(0, 2)
	sendUI(u, tcell.KeyEnter, 0)
	sendUI(u, tcell.KeyRune, ':')
	if u.app.GetFocus() != u.commandBar || u.screen != "groups" {
		t.Fatal("colon did not focus command bar on the current tab")
	}
	u.commandBar.SetText("active")
	sendUI(u, tcell.KeyEnter, 0)
	if u.screen != "active-runs" || u.commandBar.GetText() != "" {
		t.Fatal("command did not open active runs")
	}
}
func TestExitCommandRequiresEnter(t *testing.T) {
	u, _ := simulationUI(t)
	u.home()
	sendUI(u, tcell.KeyRune, ':')
	u.commandBar.SetText("exit")
	if u.ctx.Err() != nil {
		t.Fatal("typing exit quit before Enter")
	}
	sendUI(u, tcell.KeyEnter, 0)
	if u.ctx.Err() == nil {
		t.Fatal("exit command did not quit")
	}
}
func TestCommandBarPreservesNormalFieldColons(t *testing.T) {
	u, _ := simulationUI(t)
	u.contextForm()
	field := u.app.GetFocus().(*tview.InputField)
	sendUI(u, tcell.KeyRune, ':')
	if u.app.GetFocus() != field || !strings.Contains(field.GetText(), ":") {
		t.Fatal("command shortcut hijacked normal input")
	}
}
func TestUnknownTextStillFiltersPipelines(t *testing.T) {
	u, _ := simulationUI(t)
	u.pipelines = []Pipeline{{Id: 1, Name: "orders"}, {Id: 2, Name: "payments"}}
	u.home()
	sendUI(u, tcell.KeyRune, ':')
	u.commandBar.SetText("payment")
	sendUI(u, tcell.KeyEnter, 0)
	table := u.app.GetFocus().(*tview.Table)
	if table.GetRowCount() != 2 || table.GetCell(1, 2).Text != "payments" {
		t.Fatal("command bar lost pipeline filtering")
	}
}

type timedOutToken struct{}

func (timedOutToken) Authorization(context.Context) (string, error) {
	return "", context.DeadlineExceeded
}

func TestOptionsTimeoutLeavesRetryableBranchSelector(t *testing.T) {
	u, _ := simulationUI(t)
	u.client = &Client{BaseURL: "http://localhost", Org: "org", Project: "project", Tokens: timedOutToken{}, HTTP: &http.Client{}}
	ready := make(chan struct{}, 1)
	u.app.SetAfterDrawFunc(func(tcell.Screen) {
		if !u.busy && strings.Contains(u.lastError, "timed out") {
			select {
			case ready <- struct{}{}:
			default:
			}
		}
	})
	u.loadRunForm(Pipeline{Id: 7, Name: "orders"}, RunOptions{PipelineID: 7, Branch: "feat/CLI"}, []string{"main", "feat/CLI"})
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
		t.Fatal("loading message never changed to timeout")
	}
	u.app.QueueUpdateDraw(func() {
		if u.form.GetButtonIndex("Retry") < 0 || u.form.GetButtonIndex("Select stages") >= 0 {
			t.Error("timeout was not retryable or still allowed launch")
		}
		if d, ok := u.form.GetFormItem(0).(*tview.DropDown); !ok {
			t.Error("branch selector was lost")
		} else {
			_, branch := d.GetCurrentOption()
			if branch != "feat/CLI" {
				t.Error("selected branch changed")
			}
		}
		for i := 0; i < u.form.GetFormItemCount(); i++ {
			if v, ok := u.form.GetFormItem(i).(*tview.TextView); ok && strings.Contains(v.GetText(false), "Loading pipeline parameters") {
				t.Error("stale loading message remained after timeout")
			}
		}
		sendUI(u, tcell.KeyEscape, 0)
		if u.screen != "pipelines" {
			t.Error("timeout blocked back navigation")
		}
	})
}

func TestRunFailureIssuesCanBeReadAndCopiedWithoutLogs(t *testing.T) {
	u, _ := simulationUI(t)
	u.client = &Client{BaseURL: "https://dev.azure.com", Org: "example", Project: "services"}
	u.build = Build{ID: 8, Result: "failed", Definition: Pipeline{Id: 1, Name: "orders"}}
	u.records = []Record{{ID: "job", Name: "Job", Type: "Job", Result: "failed", Issues: []Issue{{Type: "error", Message: "No hosted parallelism has been purchased or granted."}}}}
	copied := ""
	u.copyText = func(_ context.Context, text string) error { copied = text; return nil }
	u.show("timeline", "Run 8", table())
	u.showRunIssues()
	view := u.app.GetFocus().(*tview.TextView)
	if !strings.Contains(view.GetText(false), "No hosted parallelism") {
		t.Fatal("job failure was not visible")
	}
	sendUI(u, tcell.KeyF2, 0)
	if !strings.Contains(copied, "No hosted parallelism") {
		t.Fatal("failure message was not copied")
	}
}

func TestRepositoryPaneFiltersPipelinesAndPreservesFocus(t *testing.T) {
	u, _ := simulationUI(t)
	u.repositories = []Repo{{Id: "one", Name: "Orders"}, {Id: "two", Name: "Payments"}, {Id: "empty", Name: "Empty"}}
	u.pipelines = []Pipeline{{Id: 1, Name: "orders build", Repository: &Repo{Id: "one", Name: "Orders"}}, {Id: 2, Name: "orders test", Repository: &Repo{Id: "one", Name: "Orders"}}, {Id: 3, Name: "payments build", Repository: &Repo{Id: "two", Name: "Payments"}}}
	u.home()
	sendUI(u, tcell.KeyLeft, 0)
	if u.app.GetFocus() != u.repoTable {
		t.Fatal("Left did not enter repository pane")
	}
	sendUI(u, tcell.KeyDown, 0)
	if u.selectedRepoID != "one" || u.pipelineTable.GetRowCount() != 3 {
		t.Fatal("Orders did not filter pipelines")
	}
	if u.app.GetFocus() != u.repoTable {
		t.Fatal("filtering stole repository focus")
	}
	sendUI(u, tcell.KeyRight, 0)
	if u.app.GetFocus() != u.pipelineTable {
		t.Fatal("Right did not enter pipelines")
	}
	sendUI(u, tcell.KeyRune, ':')
	u.commandBar.SetText("test")
	sendUI(u, tcell.KeyEnter, 0)
	if u.pipelineTable.GetRowCount() != 2 || u.pipelineTable.GetCell(1, 2).Text != "orders test" {
		t.Fatal("repository and text filters did not combine")
	}
	u.commandBar.SetText("")
	sendUI(u, tcell.KeyLeft, 0)
	sendUI(u, tcell.KeyDown, 0)
	if u.selectedRepoID != "two" || u.pipelineTable.GetRowCount() != 2 || u.pipelineTable.GetCell(1, 2).Text != "payments build" {
		t.Fatal("changing repo left stale pipelines")
	}
	sendUI(u, tcell.KeyDown, 0)
	u.app.ForceDraw()
	sendUI(u, tcell.KeyRight, 0)
	sendUI(u, tcell.KeyDown, 0)
	if u.pipelineTable.GetRowCount() != 1 {
		t.Fatal("empty repository showed unrelated pipelines")
	}
	u.home()
	if u.selectedRepoID != "empty" {
		t.Fatal("returning home lost repository scope")
	}
	u.repoTable.Select(1, 0)
	if u.pipelineTable.GetRowCount() != 4 {
		t.Fatal("All repositories did not restore all pipelines")
	}
}
func TestRepositoryPaneDoesNotAddSecondFocusHighlight(t *testing.T) {
	u, screen := simulationUI(t)
	u.pipelines = []Pipeline{{Id: 1, Name: "orders"}}
	u.home()
	u.app.ForceDraw()
	if purpleSelectionCells(screen, u.repoTable) != 0 || purpleSelectionCells(screen, u.pipelineTable) == 0 {
		t.Fatal("both panes appeared selected")
	}
	sendUI(u, tcell.KeyLeft, 0)
	u.app.ForceDraw()
	if purpleSelectionCells(screen, u.repoTable) == 0 || purpleSelectionCells(screen, u.pipelineTable) != 0 {
		t.Fatal("focus did not transfer to repository pane")
	}
}
