package pipely

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *terminalUI) newTabs() *tview.Table {
	tabs := tview.NewTable().SetSelectable(false, true)
	focusTableSelection(tabs, tcell.StyleDefault.Background(charcoal).Foreground(purple).Bold(true))
	for i, name := range []string{"Pipelines", "Active runs", "Groups"} {
		tabs.SetCell(0, i, cell("  "+name+"  "))
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
	for i := 0; i < 3; i++ {
		c := u.tabs.GetCell(0, i)
		c.SetTextColor(tcell.ColorWhite)
		c.SetAttributes(tcell.AttrNone)
		if i == active {
			c.SetTextColor(purple)
			c.SetAttributes(tcell.AttrBold)
		}
	}
	if !u.tabs.HasFocus() {
		u.tabs.Select(0, active)
	}
}
func (u *terminalUI) activeRunsScreen() {
	u.show("active-runs", "Active pipeline runs", table())
	if u.client == nil {
		u.status("Select a project and authenticate to load active runs")
		return
	}
	u.loadActiveRuns()
}
func (u *terminalUI) loadActiveRuns() {
	cl := u.client
	u.asyncRead(func(ctx context.Context) (func(), error) {
		runs, e := cl.ActiveRuns(ctx)
		return func() { u.renderActiveRuns(runs) }, e
	})
}
func (u *terminalUI) renderActiveRuns(runs []Build) {
	t := table()
	headers(t, "Run", "Pipeline", "Status", "Branch")
	for i, b := range runs {
		t.SetCell(i+1, 0, cell(strconv.Itoa(b.ID)))
		t.SetCell(i+1, 1, cell(b.Definition.Name))
		t.SetCell(i+1, 2, cell(b.State()))
		t.SetCell(i+1, 3, cell(b.SourceBranch))
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
		if e.Rune() == 'r' {
			u.loadActiveRuns()
			return nil
		}
		return vimTable(t, e)
	})
	u.replace(t)
	u.status(fmt.Sprintf("%d active runs in this project · Updates every 3 seconds.", len(runs)))
}
