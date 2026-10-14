package pipely

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *terminalUI) newTabs() *tview.Table {
	tabs := tview.NewTable().SetSelectable(false, true)
	background := tcell.NewHexColor(0x30263e)
	tabs.SetBorder(true).SetBorderColor(purple)
	tabs.SetBackgroundColor(background)
	focusTableSelection(tabs, tcell.StyleDefault.Background(background).Foreground(purple).Bold(true).Underline(true))
	for i, name := range []string{"Pipelines", "Active runs", "Groups"} {
		tabs.SetCell(0, i, cell("  "+name+"  ").SetAlign(tview.AlignCenter).SetBackgroundColor(background))
	}
	tabs.SetSelectedFunc(func(_, col int) {
		switch col {
		case 0:
			u.home()
		case 1:
			u.activeRunsScreen()
		case 2:
			u.groupScreen()
		}
		u.app.SetFocus(tabs)
	})
	tabs.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyDown || e.Key() == tcell.KeyEscape {
			if u.content != nil {
				u.app.SetFocus(u.content)
			}
			return nil
		}
		return e
	})
	return tabs
}
func (u *terminalUI) updateTabs() {
	active := 0
	switch u.screen {
	case "active-runs":
		active = 1
	case "groups", "group-detail", "group-results", "save-group":
		active = 2
	case "timeline", "logs":
		if u.runOrigin == "active-runs" {
			active = 1
		}
	}
	for i, name := range []string{"Pipelines", "Active runs", "Groups"} {
		c := u.tabs.GetCell(0, i)
		c.SetText("  " + name + "  ")
		c.SetTextColor(tcell.NewHexColor(0xc9bedb))
		c.SetAttributes(tcell.AttrBold)
		if i == active {
			c.SetText("  ● " + name + "  ")
			c.SetTextColor(purple)
			c.SetAttributes(tcell.AttrBold | tcell.AttrUnderline)
		}
	}
	if !u.tabs.HasFocus() {
		u.tabs.Select(0, active)
	}
}
func (u *terminalUI) activeRunsScreen() {
	t := table()
	u.show("active-runs", "Active pipeline runs", t)
	u.mountProjectContent(t, "Active runs · "+u.config.Project, true)
	if u.client == nil {
		u.status("Select a project and authenticate to load active runs")
		return
	}
	u.loadActiveRunsWithProjects(true)
}
func (u *terminalUI) loadActiveRuns() {
	u.loadActiveRunsWithProjects(false)
}
func (u *terminalUI) loadActiveRunsWithProjects(refreshProjects bool) {
	cl := u.client
	u.asyncRead(func(ctx context.Context) (func(), error) {
		var projects []Project
		var projectErr error
		if refreshProjects {
			projects, projectErr = cl.Projects(ctx)
		}
		runs, e := cl.ActiveRuns(ctx)
		return func() {
			if refreshProjects && projectErr == nil {
				u.projects, u.projectsOrg = projects, cl.Org
				u.renderProjects()
			}
			u.renderActiveRuns(runs)
			if projectErr != nil {
				u.status("Project listing unavailable: " + projectErr.Error())
			}
		}, e
	})
}
func (u *terminalUI) renderActiveRuns(runs []Build) {
	t := table()
	headers(t, "Run", "Pipeline", "Status", "Branch")
	for i, b := range runs {
		t.SetCell(i+1, 0, cell(strconv.Itoa(b.ID)))
		t.SetCell(i+1, 1, cell(b.Definition.Name))
		t.SetCell(i+1, 2, cell(b.State()))
		t.SetCell(i+1, 3, cell(strings.TrimPrefix(b.SourceBranch, "refs/heads/")))
	}
	t.SetSelectedFunc(func(row, _ int) {
		if row > 0 && row <= len(runs) {
			u.runOrigin = "active-runs"
			u.build = runs[row-1]
			u.pipeline = u.build.Definition
			u.timelineScreen()
		}
	})
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyLeft || e.Rune() == 'h' {
			u.app.SetFocus(u.projectTable)
			return nil
		}
		if e.Rune() == 'r' {
			u.loadActiveRunsWithProjects(true)
			return nil
		}
		return vimTable(t, e)
	})
	ids := make([]int, len(runs))
	for i, run := range runs {
		ids[i] = run.ID
	}
	u.addRunLinks(t, 4, ids)
	u.replace(t)
	u.status(fmt.Sprintf("%d active runs · %s", len(runs), u.config.Project))
}
