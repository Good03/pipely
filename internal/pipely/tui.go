package pipely

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var purple = tcell.NewHexColor(0xb694ff)
var charcoal = tcell.NewHexColor(0x171522)

type terminalUI struct {
	app                *tview.Application
	pages              *tview.Pages
	body               *tview.Flex
	header, footer     *tview.TextView
	hints              *tview.TextView
	commandBar         *tview.InputField
	renderPipelines    func()
	ctx                context.Context
	cancel             context.CancelFunc
	config             Config
	path               string
	client             *Client
	generation         int
	busy               bool
	screen             string
	screenCtx          context.Context
	screenCancel       context.CancelFunc
	pipelines          []Pipeline
	projects           []Project
	projectsOrg        string
	projectTable       *tview.Table
	projectMain        *tview.Table
	projectBelow       *tview.Table
	renderProjects     func()
	repositories       []Repo
	selectedRepoID     string
	repoTable          *tview.Table
	pipelineTable      *tview.Table
	renderRepositories func()
	repositoryNotice   string
	selected           map[int]bool
	pipeline           Pipeline
	build              Build
	records            []Record
	logID              int
	followLog          bool
	groups             []Group
	groupRuns          []GroupResult
	groupRunGroup      Group
	groupLaunchErr     error
	editingGroupName   string
	tabs               *tview.Table
	content            tview.Primitive
	runOrigin          string
	form               *tview.Form
	choosing           bool
	back               func()
	overlayFocus       map[string]tview.Primitive
	terminalScreen     tcell.Screen
	mouseEnabled       bool
	lastError          string
	failureShownFor    int
	copyText           func(context.Context, string) error
	openURL            func(context.Context, string) error
}

func newTerminalUI(parent context.Context, cancel context.CancelFunc, c Config, path string) *terminalUI {
	u := &terminalUI{app: tview.NewApplication(), ctx: parent, cancel: cancel, config: c, path: path, selected: map[int]bool{}, overlayFocus: map[string]tview.Primitive{}, copyText: nativeCopy, openURL: openBrowserLink}
	tview.Styles.PrimitiveBackgroundColor = charcoal
	tview.Styles.ContrastBackgroundColor = tcell.NewHexColor(0x30263e)
	tview.Styles.MoreContrastBackgroundColor = charcoal
	tview.Styles.BorderColor = purple
	tview.Styles.TitleColor = purple
	tview.Styles.PrimaryTextColor = tcell.NewHexColor(0xeee8fa)
	tview.Styles.SecondaryTextColor = tcell.NewHexColor(0xc9bedb)
	tview.Styles.TertiaryTextColor = tcell.NewHexColor(0xc9bedb)
	tview.Styles.InverseTextColor = charcoal
	u.header = tview.NewTextView().SetDynamicColors(true)
	u.footer = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	u.footer.SetBackgroundColor(charcoal)
	u.hints = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	u.body = tview.NewFlex()
	u.pages = tview.NewPages()
	u.tabs = u.newTabs()
	u.commandBar = u.newCommandBar()
	layout := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(u.header, 1, 0, false).AddItem(u.tabs, 3, 0, false).AddItem(u.commandBar, 1, 0, false).AddItem(u.body, 0, 1, true).AddItem(u.footer, 2, 0, false).AddItem(u.hints, 2, 0, false)
	u.pages.AddPage("main", layout, true, true)
	u.app.SetRoot(u.pages, true).EnableMouse(false)
	u.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		u.terminalScreen = screen
		u.updateHints()
		u.updateTabs()
		return false
	})
	u.app.SetInputCapture(u.input)
	return u
}

func StartTUI(parent context.Context, c Config, path string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	u := newTerminalUI(ctx, cancel, c, path)
	if c.Org == "" || c.Project == "" {
		u.contextForm()
	} else {
		u.connect()
	}
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				u.app.Stop()
				return
			case <-ticker.C:
				u.app.QueueUpdateDraw(func() {
					if front, _ := u.pages.GetFrontPage(); !u.busy && front == "main" {
						switch u.screen {
						case "runs":
							u.loadRuns()
						case "active-runs":
							u.loadActiveRuns()
						case "timeline":
							u.loadTimeline()
						case "logs":
							u.loadLog()
						case "group-results":
							u.loadGroupStatuses()
						}
					}
				})
			}
		}
	}()
	return u.app.Run()
}
func RunTui(c Config) {
	if err := StartTUI(context.Background(), c, GetConfigPath()); err != nil {
		fmt.Println(err)
	}
}
func (u *terminalUI) show(name, title string, widget tview.Primitive) {
	if u.screenCancel != nil {
		u.screenCancel()
	}
	u.screenCtx, u.screenCancel = context.WithCancel(u.ctx)
	tabsFocused := u.tabs.HasFocus()
	u.generation++
	u.busy = false
	u.screen = name
	u.content = widget
	u.form, _ = widget.(*tview.Form)
	u.choosing = u.form != nil || isChoiceScreen(name)
	u.setBack(name)
	u.title()
	u.body.Clear()
	if u.choosing {
		u.body.AddItem(newCenteredDialog(widget, title), 0, 1, true)
	} else if name == "pipelines" {
		u.body.AddItem(widget, 0, 1, true)
	} else {
		box := tview.NewFlex().AddItem(widget, 0, 1, true)
		box.SetBorder(true).SetTitle(" " + title + " ")
		u.body.AddItem(box, 0, 1, true)
	}
	prepareTable(widget)
	if tabsFocused && !u.choosing {
		u.app.SetFocus(u.tabs)
	} else {
		u.app.SetFocus(widget)
	}
}

// Read requests belong to their screen. Writes retain the application context
// so leaving a tab cannot interrupt a queue/cancel/retry submission.
func (u *terminalUI) asyncRead(work func(context.Context) (func(), error)) {
	ctx := u.screenCtx
	if ctx == nil {
		ctx = u.ctx
	}
	u.async(func() (func(), error) { return work(ctx) })
}
func (u *terminalUI) async(work func() (func(), error)) {
	if u.busy {
		return
	}
	u.busy = true
	generation := u.generation
	if u.screen != "runs" && u.screen != "active-runs" && u.screen != "timeline" && u.screen != "logs" && u.screen != "group-results" {
		u.status("Loading…")
	}
	go func() {
		apply, err := work()
		if u.ctx.Err() != nil {
			return
		}
		u.app.QueueUpdateDraw(func() {
			if generation != u.generation {
				return
			}
			u.busy = false
			if err != nil {
				u.reportError(err)
				return
			}
			u.status("Ready")
			if apply != nil {
				apply()
			}
		})
	}()
}
func (u *terminalUI) connect() {
	cl, e := NewClient(u.config)
	if e != nil {
		u.authForm()
		u.reportError(e)
		return
	}
	u.client = cl
	u.home()
	u.refreshPipelines()
}
func table() *tview.Table {
	t := tview.NewTable().SetSelectable(false, false).SetFixed(1, 0)
	focusTableSelection(t, tcell.StyleDefault.Background(charcoal).Foreground(tview.Styles.PrimaryTextColor))
	return t
}
func cell(s string) *tview.TableCell { return tview.NewTableCell(tview.Escape(s)).SetExpansion(1) }
func headers(t *tview.Table, names ...string) {
	for i, n := range names {
		t.SetCell(0, i, cell(n).SetTextColor(purple).SetSelectable(false))
	}
}
func vimTable(t *tview.Table, e *tcell.EventKey) *tcell.EventKey {
	switch e.Rune() {
	case 'j':
		return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	case 'k':
		return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	case 'h':
		return tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)
	case 'l':
		return tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)
	}
	return e
}
func (u *terminalUI) home() {
	u.editingGroupName = ""
	t := table()
	bar := u.commandBar
	repoTable := table()
	u.repoTable = repoTable
	u.pipelineTable = t
	projectTable := u.newProjectPane(t, repoTable)
	repoTable.SetBorder(true).SetTitle(" Repositories ")
	t.SetBorder(true).SetTitle(" Pipelines ")
	render := func() {
		t.Clear()
		headers(t, "Group", "ID", "Pipeline")
		for _, p := range filterRepositoryPipelines(u.pipelines, u.selectedRepoID) {
			if !strings.Contains(strings.ToLower(p.Name), strings.ToLower(bar.GetText())) {
				continue
			}
			mark := "[ ]"
			if u.selected[p.Id] {
				mark = "[x]"
			}
			row := t.GetRowCount()
			t.SetCell(row, 0, cell(mark))
			t.SetCell(row, 1, cell(strconv.Itoa(p.Id)))
			t.SetCell(row, 2, cell(p.Name).SetReference(p))
		}
		title := "All repositories"
		for _, r := range u.repositories {
			if r.Id == u.selectedRepoID {
				title = r.Name
			}
		}
		t.SetTitle(" Pipelines · " + tview.Escape(title) + " ")
		prepareTable(t)
		if t.GetRowCount() > 1 {
			t.Select(1, 0)
		}
	}
	u.renderPipelines = render
	u.renderRepositories = func() {
		repoTable.Clear()
		headers(repoTable, "Repository")
		repoTable.SetCell(1, 0, cell("All repositories").SetReference(""))
		selectedRow := 1
		for i, r := range u.repositories {
			repoTable.SetCell(i+2, 0, cell(r.Name).SetReference(r.Id))
			if r.Id == u.selectedRepoID {
				selectedRow = i + 2
			}
		}
		if selectedRow == 1 {
			u.selectedRepoID = ""
		}
		prepareTable(repoTable)
		repoTable.Select(selectedRow, 0)
	}
	repoTable.SetSelectionChangedFunc(func(row, _ int) {
		if row < 1 || row >= repoTable.GetRowCount() {
			return
		}
		id, ok := repoTable.GetCell(row, 0).GetReference().(string)
		if !ok {
			return
		}
		u.selectedRepoID = id
		render()
	})
	repoTable.SetSelectedFunc(func(row, col int) {
		if row > 0 {
			u.selectedRepoID, _ = repoTable.GetCell(row, col).GetReference().(string)
			render()
		}
	})
	repoTable.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		row, _ := repoTable.GetSelection()
		if row <= 1 && (e.Key() == tcell.KeyUp || e.Rune() == 'k') {
			u.app.SetFocus(projectTable)
			return nil
		}
		if e.Key() == tcell.KeyRight || e.Key() == tcell.KeyEscape || e.Rune() == 'l' {
			u.app.SetFocus(t)
			return nil
		}
		if e.Rune() == 'r' {
			u.refreshPipelines()
			return nil
		}
		return vimTable(repoTable, e)
	})
	current := func() (Pipeline, bool) {
		row, _ := t.GetSelection()
		if row < 1 || row >= t.GetRowCount() {
			return Pipeline{}, false
		}
		p, ok := t.GetCell(row, 2).GetReference().(Pipeline)
		return p, ok
	}
	t.SetSelectedFunc(func(int, int) {
		if p, ok := current(); ok {
			u.pipeline = p
			u.runsScreen()
		}
	})
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyLeft || e.Rune() == 'h' {
			u.app.SetFocus(repoTable)
			return nil
		}
		switch e.Rune() {
		case '/':
			u.app.SetFocus(bar)
			return nil
		case ' ':
			if p, ok := current(); ok {
				u.selected[p.Id] = !u.selected[p.Id]
				render()
			}
			return nil
		case 'n':
			if p, ok := current(); ok {
				u.runForm(p, RunOptions{PipelineID: p.Id})
			}
			return nil
		case 's':
			u.saveSelectedGroup()
			return nil
		case 'g':
			u.groupScreen()
			return nil
		case 'o':
			u.contextScreen()
			return nil
		case 'a':
			u.authForm()
			return nil
		case 'p':
			u.approvalScreen()
			return nil
		case 'r':
			u.refreshPipelines()
			return nil
		}
		return vimTable(t, e)
	})
	sidebar := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(projectTable, 0, 1, false).AddItem(repoTable, 0, 2, true)
	panes := tview.NewFlex().AddItem(sidebar, 28, 0, false).AddItem(t, 0, 1, true)
	u.show("pipelines", "Pipelines", panes)
	if !u.tabs.HasFocus() {
		u.app.SetFocus(t)
	}
	u.renderRepositories()
	render()
	u.status("Ready")
}
func (u *terminalUI) refreshPipelines() {
	cl := u.client
	u.asyncRead(func(ctx context.Context) (func(), error) {
		projects, projectErr := cl.Projects(ctx)
		catalog, e := cl.Catalog(ctx)
		return func() {
			if projectErr == nil {
				u.projects, u.projectsOrg = projects, cl.Org
				if u.renderProjects != nil {
					u.renderProjects()
				}
			}
			if e != nil {
				u.reportError(e)
				return
			}
			u.pipelines = catalog.Pipelines
			u.repositories = catalog.Repositories
			u.repositoryNotice = catalog.Notice
			if u.renderRepositories != nil {
				u.renderRepositories()
			}
			if u.renderPipelines != nil {
				u.renderPipelines()
			}
			if projectErr != nil {
				u.status("Project listing unavailable: " + projectErr.Error())
			} else if catalog.Notice != "" {
				u.status(catalog.Notice)
			} else {
				u.status(fmt.Sprintf("%d repositories · %d pipelines in this project", len(u.repositories), len(u.pipelines)))
			}
		}, nil
	})
}
func (u *terminalUI) contextScreen() {
	list := tview.NewList().SetSelectedFocusOnly(true).ShowSecondaryText(false)
	for _, x := range u.config.Contexts {
		x := x
		list.AddItem(x.Org+" / "+x.Project, "", 0, func() {
			u.config.RememberContext(x.Org, x.Project)
			if e := u.saveSettings(); e != nil {
				u.reportError(e)
				return
			}
			u.selected = map[int]bool{}
			u.pipelines = nil
			u.repositories = nil
			u.selectedRepoID = ""
			u.connect()
		})
	}
	u.show("contexts", "Organization / Project", list)
	actions := tview.NewForm().AddButton("Add organization", u.contextForm)
	list.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Rune() == 'n' {
			u.contextForm()
			return nil
		}
		if (e.Key() == tcell.KeyDown || e.Rune() == 'j' || e.Key() == tcell.KeyTab) && list.GetCurrentItem() >= list.GetItemCount()-1 {
			u.app.SetFocus(actions)
			return nil
		}
		return e
	})
	actions.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if (e.Key() == tcell.KeyUp || e.Rune() == 'k') && list.GetItemCount() > 0 {
			u.app.SetFocus(list)
			return nil
		}
		if e.Rune() == 'n' {
			u.contextForm()
			return nil
		}
		return e
	})
	u.body.Clear().AddItem(newCenteredDialog(newDialogActions(list, actions), "Organization / Project"), 0, 1, true)
	if list.GetItemCount() == 0 {
		u.app.SetFocus(actions)
	}
}
func (u *terminalUI) contextForm() {
	org := u.config.Org
	form := tview.NewForm()
	form.AddInputField("Organization", org, 50, nil, func(s string) { org = s })
	form.AddButton("Load projects", func() {
		org = strings.TrimSpace(strings.TrimPrefix(org, "https://dev.azure.com/"))
		org = strings.Trim(org, "/")
		if org == "" || strings.ContainsAny(org, "/?#") {
			u.status("Enter an organization name")
			return
		}
		c := u.config
		c.Org = org
		c.Project = "_"
		cl, e := NewClient(c)
		if e != nil {
			u.config.Org = org
			u.authForm()
			u.reportError(e)
			return
		}
		u.async(func() (func(), error) {
			ps, e := cl.Projects(u.ctx)
			return func() {
				list := tview.NewList().SetSelectedFocusOnly(true).ShowSecondaryText(false)
				for _, p := range ps {
					p := p
					list.AddItem(p.Name, "", 0, func() {
						u.config.RememberContext(org, p.Name)
						if e := u.saveSettings(); e != nil {
							u.reportError(e)
							return
						}
						u.selected = map[int]bool{}
						u.pipelines = nil
						u.repositories = nil
						u.selectedRepoID = ""
						u.connect()
					})
				}
				u.show("projects", "Select project", list)
			}, e
		})
	})
	form.AddButton("Authentication", u.authForm)
	form.AddButton("Back", u.contextScreen)
	u.show("context-form", "Organization", form)
}
func (u *terminalUI) authForm() {
	form := tview.NewForm()
	pat := ""
	mode := u.config.Auth
	if mode == "" {
		mode = "az"
	}
	form.AddDropDown("Authentication", []string{"az", "pat"}, map[string]int{"az": 0, "pat": 1}[mode], func(s string, _ int) { mode = s })
	form.AddInputField("PAT (optional; otherwise use environment/store)", "", 40, nil, func(s string) { pat = s })
	form.GetFormItem(1).(*tview.InputField).SetMaskCharacter('*')
	form.AddButton("Use authentication", func() {
		if mode == "pat" && pat != "" {
			if e := StorePAT(u.config.Org, pat); e != nil {
				u.reportError(e)
				return
			}
			u.config.PAT = ""
			u.config.LegacyPATOrg = ""
		}
		u.config.Auth = mode
		if e := u.saveSettings(); e != nil {
			u.reportError(e)
			return
		}
		if u.config.Project == "" {
			u.contextForm()
		} else {
			u.connect()
		}
	})
	form.AddButton("Azure login", func() {
		u.app.Suspend(func() {
			if e := AzureLogin(u.ctx, false); e != nil {
				fmt.Println(e)
			}
		})
		u.status("Azure login finished; select az and Use authentication")
	})
	form.AddButton("Device login", func() {
		u.app.Suspend(func() {
			if e := AzureLogin(u.ctx, true); e != nil {
				fmt.Println(e)
			}
		})
		u.status("Azure login finished; select az and Use authentication")
	})
	form.AddButton("Back", func() {
		if u.client != nil {
			u.home()
		} else {
			u.contextForm()
		}
	})
	u.show("auth", "Authentication", form)
}
func (u *terminalUI) runsScreen() {
	u.runOrigin = "runs"
	u.show("runs", "Recent runs · "+tview.Escape(u.pipeline.Name), table())
	u.loadRuns()
}
func (u *terminalUI) loadRuns() {
	cl, id := u.client, u.pipeline.Id
	u.async(func() (func(), error) {
		runs, e := cl.Runs(u.ctx, id)
		return func() {
			t := table()
			headers(t, "Run", "Number", "Status", "Branch")
			for i, b := range runs {
				t.SetCell(i+1, 0, cell(strconv.Itoa(b.ID)))
				t.SetCell(i+1, 1, cell(b.BuildNumber))
				t.SetCell(i+1, 2, cell(b.State()))
				t.SetCell(i+1, 3, cell(b.SourceBranch))
			}
			t.SetSelectedFunc(func(row, _ int) {
				if row > 0 && row <= len(runs) {
					u.build = runs[row-1]
					u.timelineScreen()
				}
			})
			t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
				if e.Key() == tcell.KeyEscape {
					u.home()
					return nil
				}
				switch e.Rune() {
				case 'n':
					u.runForm(u.pipeline, RunOptions{PipelineID: id})
					return nil
				case 'r':
					u.loadRuns()
					return nil
				}
				return vimTable(t, e)
			})
			u.replace(t)
			u.status("Latest five runs + active runs")
		}, e
	})
}
func (u *terminalUI) replace(widget tview.Primitive) { // Refresh without changing the navigation generation.
	tabsFocused := u.app.GetFocus() == u.tabs
	projectsFocused := u.screen == "active-runs" && u.app.GetFocus() == u.projectTable
	u.content = widget
	prepareTable(widget)
	oldRow := 1
	if old, ok := u.app.GetFocus().(*tview.Table); ok && !tabsFocused {
		oldRow, _ = old.GetSelection()
	}
	if u.screen == "active-runs" {
		u.mountProjectContent(widget.(*tview.Table), "Active runs · "+u.config.Project, false)
	} else {
		u.body.Clear()
		box := tview.NewFlex().AddItem(widget, 0, 1, true)
		box.SetBorder(true).SetTitle(" " + u.screen + " ")
		u.body.AddItem(box, 0, 1, true)
	}
	if t, ok := widget.(*tview.Table); ok && t.GetRowCount() > 1 {
		if oldRow >= t.GetRowCount() {
			oldRow = t.GetRowCount() - 1
		}
		t.Select(oldRow, 0)
	}
	if !tabsFocused && !projectsFocused {
		u.app.SetFocus(widget)
	}
}
func (u *terminalUI) timelineScreen() {
	u.show("timeline", fmt.Sprintf("Run %d", u.build.ID), table())
	u.loadTimeline()
}
func orderedRecords(records []Record) []Record {
	byParent := map[string][]Record{}
	ids := map[string]bool{}
	for _, r := range records {
		ids[r.ID] = true
	}
	for _, r := range records {
		parent := r.ParentID
		if !ids[parent] {
			parent = ""
		}
		byParent[parent] = append(byParent[parent], r)
	}
	for parent := range byParent {
		sort.SliceStable(byParent[parent], func(i, j int) bool { return byParent[parent][i].Order < byParent[parent][j].Order })
	}
	result := []Record{}
	seen := map[string]bool{}
	var walk func(string, int)
	walk = func(parent string, depth int) {
		for _, r := range byParent[parent] {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			r.Name = strings.Repeat("  ", depth) + r.Name
			result = append(result, r)
			walk(r.ID, depth+1)
		}
	}
	walk("", 0)
	return result
}
func (u *terminalUI) loadTimeline() {
	cl, id := u.client, u.build.ID
	u.async(func() (func(), error) {
		b, e := cl.Build(u.ctx, id)
		if e != nil {
			return nil, e
		}
		records, e := cl.Timeline(u.ctx, id)
		if e != nil {
			return func() {
				u.build = b
				u.records = nil
				d := RunDiagnostics{Run: b, TimelineError: e.Error(), URL: cl.RunURL(id)}
				u.lastError = d.Report()
				u.info("Run diagnostics · F2 copy", d.Report())
			}, nil
		}
		approvals, approvalErr := cl.Approvals(u.ctx)
		return func() {
			u.build = b
			u.records = orderedRecords(records)
			t := table()
			headers(t, "Type", "Stage / Job / Task", "Status", "Attempt", "Log")
			for i, r := range u.records {
				t.SetCell(i+1, 0, cell(r.Type))
				t.SetCell(i+1, 1, cell(r.Name))
				t.SetCell(i+1, 2, cell(r.Status()))
				t.SetCell(i+1, 3, cell(strconv.Itoa(r.Attempt)))
				log := ""
				if r.Log != nil {
					log = strconv.Itoa(r.Log.ID)
				}
				t.SetCell(i+1, 4, cell(log))
			}
			t.SetSelectedFunc(func(row, _ int) {
				if row > 0 && row <= len(u.records) {
					r := u.records[row-1]
					if r.Log != nil {
						u.logID = r.Log.ID
						u.logScreen()
					} else if len(r.Issues) > 0 || r.Result == "failed" || r.ErrorCount > 0 {
						u.showRunIssues()
					} else {
						u.status("Select a task with a log ID; job output is listed under its tasks")
					}
				}
			})
			t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
				if e.Key() == tcell.KeyEscape {
					u.runsScreen()
					return nil
				}
				switch e.Rune() {
				case 'e':
					u.showRunIssues()
					return nil
				case 'c':
					u.confirm("Cancel run "+strconv.Itoa(id)+"?", func() {
						u.async(func() (func(), error) { e := cl.Cancel(u.ctx, id); return func() { u.timelineScreen() }, e })
					})
					return nil
				case 'R':
					row, _ := t.GetSelection()
					stage := ""
					if row > 0 && row <= len(u.records) {
						r := u.records[row-1]
						if r.Type == "Stage" {
							stage = r.Identifier
						}
					}
					if b.Status != "completed" {
						u.status("Retry is available after a run completes")
						return nil
					}
					if b.Result != "failed" && b.Result != "partiallySucceeded" {
						u.status("Select a failed run to retry")
						return nil
					}
					target := "failed work in run"
					if stage != "" {
						target = "failed jobs in " + stage
					}
					u.confirm("Retry "+target+"?", func() {
						u.async(func() (func(), error) { e := cl.Retry(u.ctx, id, stage); return func() { u.timelineScreen() }, e })
					})
					return nil
				case 'p':
					u.approvalScreen()
					return nil
				case 'r':
					u.loadTimeline()
					return nil
				}
				return vimTable(t, e)
			})
			u.replace(t)
			waiting := []string{}
			for _, r := range records {
				if (strings.Contains(strings.ToLower(r.Type), "approval") || strings.Contains(strings.ToLower(r.Name), "approval")) && r.State != "completed" {
					waiting = append(waiting, r.Name)
				}
			}
			msg := fmt.Sprintf("Run %d: %s · Updates every 3 seconds.", id, b.State())
			if len(waiting) > 0 {
				msg = "APPROVAL REQUIRED: " + strings.Join(waiting, ", ") + " · p approvers\n" + cl.RunURL(id)
			}
			if len(waiting) == 0 && len(approvals) > 0 {
				msg = fmt.Sprintf("%d pending approvals in this project · p inspect (may belong to other runs)\n%s", len(approvals), cl.RunURL(id))
			}
			if approvalErr != nil {
				msg += " · Approval visibility unavailable"
			}
			u.status(msg)
			if b.Result == "failed" && u.failureShownFor != id {
				u.failureShownFor = id
				u.showRunIssues()
			}
		}, e
	})
}
func (u *terminalUI) logScreen() {
	u.followLog = true
	v := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(true)
	v.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			u.timelineScreen()
			return nil
		}
		if e.Rune() == 'o' {
			u.linkPicker(v.GetText(true))
			return nil
		}
		if e.Rune() == 'e' {
			u.showRunIssues()
			return nil
		}
		if e.Rune() == 'f' {
			u.followLog = !u.followLog
			if u.followLog {
				v.ScrollToEnd()
			}
			u.status("Log follow toggled.")
			return nil
		}
		return e
	})
	u.show("logs", fmt.Sprintf("Run %d · Log %d · following", u.build.ID, u.logID), v)
	u.loadLog()
}
func (u *terminalUI) loadLog() {
	cl, id, log := u.client, u.build.ID, u.logID
	u.async(func() (func(), error) {
		text, e := cl.Log(u.ctx, id, log, 0)
		return func() {
			if v, ok := u.app.GetFocus().(*tview.TextView); ok {
				v.SetText(styledLog(text))
				if u.followLog {
					v.ScrollToEnd()
				}
			}
			mode := "Reading log"
			if u.followLog {
				mode = "Following log"
			}
			u.status(mode)
		}, e
	})
}
func (u *terminalUI) approvalScreen() {
	cl := u.client
	u.show("approvals", "Pending approvals · current project", tview.NewTextView())
	u.async(func() (func(), error) {
		as, e := cl.Approvals(u.ctx)
		return func() {
			v := tview.NewTextView().SetScrollable(true)
			var b strings.Builder
			b.WriteString("Pending approvals in this project (may belong to other runs):\n\n")
			for _, a := range as {
				fmt.Fprintf(&b, "%s · %s\n%s\n", a.ID, a.Status, a.Instructions)
				for _, s := range a.Steps {
					fmt.Fprintf(&b, "  Approver: %s · %s\n", s.AssignedApprover.DisplayName, s.Status)
				}
				b.WriteString("\n")
			}
			if len(as) == 0 {
				b.WriteString("No pending approvals. Other checks may still be waiting.\n")
			}
			v.SetText(b.String())
			v.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
				if e.Key() == tcell.KeyEscape {
					if u.build.ID > 0 {
						u.timelineScreen()
					} else {
						u.home()
					}
					return nil
				}
				if e.Rune() == 'r' {
					u.approvalScreen()
					return nil
				}
				return e
			})
			u.replace(v)
			u.status("Pending approvals for this project. Complete approval in Azure DevOps.")
		}, e
	})
}
func (u *terminalUI) confirm(text string, yes func()) {
	u.messageDialog("confirm", "Confirm", text, []string{"Confirm", "Back"}, func(index int) {
		if index == 0 {
			yes()
		}
	})
}
func (u *terminalUI) stageScreen(p Pipeline, plan Plan, previous []string) {
	t := table()
	editingMember := u.editingGroupName != ""
	explicit := map[string]bool{}
	for _, id := range previous {
		explicit[id] = true
	}
	if len(previous) == 0 && len(plan.Stages) > 0 {
		if editingMember {
			// An unset group selection means all stages; preserve that default.
			for _, stage := range plan.Stages {
				explicit[stage.ID] = true
			}
		} else {
			explicit[plan.Stages[0].ID] = true
		}
	}
	included := map[string]bool{}
	render := func() {
		selected := []string{}
		for _, s := range plan.Stages {
			if explicit[s.ID] {
				selected = append(selected, s.ID)
			}
		}
		inc, _, _ := ResolveStages(plan.Stages, selected)
		included = map[string]bool{}
		for _, s := range inc {
			included[s] = true
		}
		row, _ := t.GetSelection()
		t.Clear()
		headers(t, "Run", "Stage ID", "Name", "Dependencies", "Condition")
		for i, s := range plan.Stages {
			mark := "[ ]"
			if included[s.ID] {
				mark = "[x]"
				if !explicit[s.ID] {
					mark += " required"
				}
			}
			t.SetCell(i+1, 0, cell(mark))
			t.SetCell(i+1, 1, cell(s.ID))
			t.SetCell(i+1, 2, cell(s.Name))
			t.SetCell(i+1, 3, cell(strings.Join(s.Dependencies, ", ")))
			t.SetCell(i+1, 4, cell(s.Condition))
		}
		prepareTable(t)
		if row < 1 {
			row = 1
		}
		if len(plan.Stages) > 0 {
			t.Select(row, 0)
		} else {
			t.SetCell(1, 0, cell("Implicit single stage: all jobs will run"))
		}
	}
	submit := func(save bool) {
		selected := []string{}
		for _, s := range plan.Stages {
			if included[s.ID] {
				selected = append(selected, s.ID)
			}
		}
		if len(plan.Stages) > 0 && len(selected) == 0 {
			u.status("Select at least one stage")
			return
		}
		options := plan.Options
		options.Stages = selected
		if save || editingMember {
			u.saveMemberForm(options)
			return
		}
		cl := u.client
		u.async(func() (func(), error) {
			fresh, e := cl.Prepare(u.ctx, options)
			return func() {
				text := fmt.Sprintf("Launch %s?\nBranch: %s\nStages: %s", p.Name, options.Branch, strings.Join(fresh.Included, ", "))
				u.confirm(text, func() {
					u.async(func() (func(), error) {
						b, e := cl.Launch(u.ctx, fresh)
						return func() { u.pipeline = p; u.build = b; u.timelineScreen() }, e
					})
				})
			}, e
		})
	}
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			u.runForm(p, plan.Options)
			return nil
		}
		if e.Key() == tcell.KeyEnter {
			submit(false)
			return nil
		}
		switch e.Rune() {
		case ' ':
			row, _ := t.GetSelection()
			if row > 0 && row <= len(plan.Stages) {
				s := plan.Stages[row-1]
				if included[s.ID] {
					trial := []string{}
					for id, on := range explicit {
						if on && id != s.ID {
							trial = append(trial, id)
						}
					}
					inc, _, _ := ResolveStages(plan.Stages, trial)
					for _, id := range inc {
						if id == s.ID {
							u.status("Required by another selected stage or YAML isSkippable:false; deselect dependent stages first")
							return nil
						}
					}
					delete(explicit, s.ID)
				} else {
					explicit[s.ID] = true
				}
				render()
			}
			return nil
		case 'a':
			for _, s := range plan.Stages {
				explicit[s.ID] = true
			}
			render()
			return nil
		case 'x':
			explicit = map[string]bool{}
			render()
			return nil
		case 's':
			submit(true)
			return nil
		}
		return vimTable(t, e)
	})
	u.show("stages", "Select stages · "+tview.Escape(p.Name), t)
	render()
	if editingMember {
		u.status("Editing group " + u.editingGroupName)
	} else {
		u.status("Select stages")
	}
}
func (u *terminalUI) saveSelectedGroup() {
	members := []RunOptions{}
	for _, p := range u.pipelines {
		if u.selected[p.Id] {
			members = append(members, RunOptions{PipelineID: p.Id})
		}
	}
	if len(members) == 0 {
		u.status("Select group members with Space first")
		return
	}
	u.groupNameForm(members, false)
}
func (u *terminalUI) saveMemberForm(opts RunOptions) {
	if u.editingGroupName != "" {
		g, err := FindGroup(u.config, u.editingGroupName)
		if err != nil {
			u.reportError(err)
			return
		}
		g.Members = append([]RunOptions(nil), g.Members...)
		for i, member := range g.Members {
			if member.PipelineID == opts.PipelineID {
				g.Members[i] = opts
				break
			}
		}
		if err := u.persistGroup(g); err != nil {
			u.reportError(err)
			return
		}
		u.groupDetail(g)
		u.status("Member options saved to group " + g.Name)
		return
	}
	u.groupNameForm([]RunOptions{opts}, true)
}
func (u *terminalUI) groupNameForm(members []RunOptions, appendMember bool) {
	name := ""
	f := tview.NewForm().AddInputField("Group name", "", 40, nil, func(s string) { name = s })
	f.AddTextView("Project", u.config.Org+" / "+u.config.Project, 40, 1, false, false)
	f.AddButton("Save", func() {
		g := Group{Name: strings.TrimSpace(name), Org: u.config.Org, Project: u.config.Project, Members: members}
		if !appendMember {
			if old, err := FindGroup(u.config, g.Name); err == nil {
				g.Members = preserveGroupOptions(old.Members, members)
			}
		}
		if appendMember {
			if old, e := FindGroup(u.config, g.Name); e == nil {
				g = old
				g.Members = append([]RunOptions(nil), old.Members...)
				found := false
				for i, m := range g.Members {
					if m.PipelineID == members[0].PipelineID {
						g.Members[i] = members[0]
						found = true
					}
				}
				if !found {
					g.Members = append(g.Members, members[0])
				}
			}
		}
		if e := u.persistGroup(g); e != nil {
			u.reportError(e)
			return
		}
		u.groupScreen()
	})
	f.AddButton("Back", u.home)
	u.show("save-group", "Save project group", f)
	u.status("Save group · " + u.config.Project)
}
func (u *terminalUI) groupScreen() {
	u.editingGroupName = ""
	u.groups = nil
	for _, g := range u.config.Groups {
		if g.Org == u.config.Org && g.Project == u.config.Project {
			u.groups = append(u.groups, g)
		}
	}
	t := table()
	headers(t, "Group", "Project", "Pipelines")
	for i, g := range u.groups {
		ids := []string{}
		for _, m := range g.Members {
			ids = append(ids, strconv.Itoa(m.PipelineID))
		}
		t.SetCell(i+1, 0, cell(g.Name))
		t.SetCell(i+1, 1, cell(g.Project))
		t.SetCell(i+1, 2, cell(strings.Join(ids, ", ")))
	}
	t.SetSelectedFunc(func(row, _ int) {
		if row > 0 && row <= len(u.groups) {
			u.groupDetail(u.groups[row-1])
		}
	})
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyLeft || e.Rune() == 'h' {
			u.app.SetFocus(u.projectTable)
			return nil
		}
		if e.Rune() == 'r' {
			u.refreshProjectList()
			return nil
		}
		if e.Key() == tcell.KeyEscape {
			u.home()
			return nil
		}
		return vimTable(t, e)
	})
	u.show("groups", "Project groups", t)
	u.mountProjectContent(t, "Groups · "+u.config.Project, true)
	u.status(fmt.Sprintf("%d groups · %s", len(u.groups), u.config.Project))
	u.refreshProjectList()
}
func (u *terminalUI) groupResults(g Group, results []GroupResult, launchErr error) {
	u.groupRuns = results
	u.groupRunGroup = g
	u.groupLaunchErr = launchErr
	u.renderGroupResults(nil, true)
}
func (u *terminalUI) renderGroupResults(states map[int]string, initial bool) {
	g, results, launchErr := u.groupRunGroup, u.groupRuns, u.groupLaunchErr
	t := table()
	headers(t, "Pipeline", "Run", "Submission")
	for i, r := range results {
		t.SetCell(i+1, 0, cell(strconv.Itoa(r.PipelineID)))
		t.SetCell(i+1, 1, cell(strconv.Itoa(r.RunID)))
		s := "Queued"
		if state := states[r.RunID]; state != "" {
			s = state
		}
		if r.Error != "" {
			s = r.Error
		}
		t.SetCell(i+1, 2, cell(s))
	}
	t.SetSelectedFunc(func(row, _ int) {
		if row > 0 && row <= len(results) && results[row-1].RunID > 0 {
			r := results[row-1]
			u.pipeline = Pipeline{Id: r.PipelineID}
			u.build = Build{ID: r.RunID}
			u.timelineScreen()
		}
	})
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			u.groupScreen()
			return nil
		}
		return vimTable(t, e)
	})
	if initial {
		u.show("group-results", "Group · "+tview.Escape(g.Name), t)
	} else {
		u.replace(t)
	}
	if launchErr != nil {
		u.status(launchErr.Error() + " · Check run history before resubmitting.")
	} else {
		u.status("Group queued")
	}
}
func (u *terminalUI) help() {
	u.messageDialog("help", "Help", "PIPELY\n\nArrows: fields/buttons · Enter: open/select\nSpace: checkbox · Esc: close/back · Ctrl-C: quit\nF4: tabs · Left/Right: choose tab · Down: content\nF2: copy text/row · Ctrl-Y: copy full error\nF3: toggle mouse capture (off enables text selection)\n\nPress : for the command bar (Shift+;)\nCommands: exit, help, pipelines, active, groups, refresh\nPipelines: Left/Right repository and pipeline panes\n/ command bar · n new run · r refresh\nSpace: select members · s save group · g groups\no organization/project · a auth · p approvals\n\nStages: Space toggle · a all · x clear\nEnter: review run / save group member\ns: save to group\n\nRun: Enter logs · e failure details · c cancel · R retry failed\nLogs and diagnostics: o opens links in your browser.\nNative terminal links support Ctrl/Cmd+click where available.\nLive status and logs refresh every 3 seconds.", []string{"Close"}, nil)
}

func (u *terminalUI) info(title, text string) {
	focus := u.app.GetFocus()
	u.overlayFocus["info"] = focus
	view := tview.NewTextView().SetDynamicColors(true).SetText(hyperlinkText(text)).SetScrollable(true)
	title = strings.TrimSuffix(title, " · F2 copy")
	view.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Rune() == 'o' {
			u.linkPicker(text)
			return nil
		}
		if e.Key() == tcell.KeyEscape {
			u.pages.RemovePage("info")
			u.app.SetFocus(focus)
			return nil
		}
		return e
	})
	u.pages.AddPage("info", newCenteredDialog(view, title), true, true)
	u.app.SetFocus(view)
}

func (u *terminalUI) loadGroupStatuses() {
	cl := u.client
	results := append([]GroupResult(nil), u.groupRuns...)
	u.async(func() (func(), error) {
		states := map[int]string{}
		for _, r := range results {
			if r.RunID <= 0 {
				continue
			}
			b, e := cl.Build(u.ctx, r.RunID)
			if e != nil {
				states[r.RunID] = "Status unavailable"
			} else {
				states[r.RunID] = b.State()
			}
		}
		return func() { u.renderGroupResults(states, false) }, nil
	})
}

func (u *terminalUI) saveSettings() error {
	if e := SaveSettings(u.path, u.config); e != nil {
		return e
	}
	u.config.PAT = ""
	u.config.LegacyPATOrg = ""
	u.config.Version = 2
	return nil
}

// tview can loop when navigating an all-unselectable table whose draw advanced
// its selected row beyond the header. Empty views must not enable row selection.
func prepareTable(widget tview.Primitive) {
	t, ok := widget.(*tview.Table)
	if !ok {
		return
	}
	hasRows := t.GetRowCount() > 1
	t.SetSelectable(hasRows, false)
	if hasRows {
		row, col := t.GetSelection()
		if row < 1 || row >= t.GetRowCount() {
			row = 1
		}
		if col < 0 || col >= t.GetColumnCount() {
			col = 0
		}
		t.Select(row, col)
	}
}

func (u *terminalUI) showRunIssues() {
	d := RunDiagnostics{Run: u.build, Records: u.records, URL: u.client.RunURL(u.build.ID)}
	report := d.Report()
	u.lastError = report
	u.info("Run diagnostics · F2 copy", report)
}
