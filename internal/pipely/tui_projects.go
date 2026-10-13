package pipely

import (
	"context"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *terminalUI) newProjectPane(pipelines, repositories *tview.Table) *tview.Table {
	if repositories == nil {
		repositories = pipelines
	}
	u.projectMain, u.projectBelow = pipelines, repositories
	t := table()
	u.projectTable = t
	t.SetBorder(true).SetTitle(" Projects ")
	u.renderProjects = func() {
		t.Clear()
		headers(t, "Project")
		var projects []Project
		if u.projectsOrg == u.config.Org {
			projects = append(projects, u.projects...)
		}
		// Keep the active context usable while discovery loads or is unavailable.
		found := false
		for _, p := range projects {
			found = found || p.Name == u.config.Project || p.Id == u.config.Project
		}
		if !found && u.config.Project != "" {
			projects = append(projects, Project{Name: u.config.Project})
		}
		sort.Slice(projects, func(i, j int) bool {
			return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
		})
		selected := 1
		for i, p := range projects {
			label := p.Name
			if p.Name == u.config.Project || p.Id == u.config.Project {
				label = "● " + label
				selected = i + 1
			}
			t.SetCell(i+1, 0, cell(label).SetReference(p))
		}
		prepareTable(t)
		if len(projects) > 0 {
			t.Select(selected, 0)
		}
	}
	t.SetSelectedFunc(func(row, _ int) {
		if row < 1 || row >= t.GetRowCount() {
			return
		}
		p, ok := t.GetCell(row, 0).GetReference().(Project)
		if !ok {
			return
		}
		if p.Name == u.config.Project || p.Id == u.config.Project {
			u.app.SetFocus(u.projectBelow)
			return
		}
		u.switchProject(p.Name)
	})
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		row, _ := t.GetSelection()
		switch {
		case e.Key() == tcell.KeyRight || e.Rune() == 'l':
			u.app.SetFocus(u.projectMain)
			return nil
		case e.Key() == tcell.KeyEscape:
			u.app.SetFocus(u.projectBelow)
			return nil
		case (e.Key() == tcell.KeyDown || e.Rune() == 'j') && row == t.GetRowCount()-1:
			u.app.SetFocus(u.projectBelow)
			return nil
		case e.Rune() == 'r':
			switch u.screen {
			case "groups":
				u.refreshProjectList()
			case "active-runs":
				u.loadActiveRunsWithProjects(true)
			default:
				u.refreshPipelines()
			}
			return nil
		}
		return vimTable(t, e)
	})
	u.renderProjects()
	return t
}

func (u *terminalUI) switchProject(name string) {
	screen := u.screen
	previous := u.config
	u.config.RememberContext(u.config.Org, name)
	if err := u.saveSettings(); err != nil {
		u.config = previous
		u.reportError(err)
		return
	}
	// A project change invalidates both selections and project-scoped data.
	u.selected = map[int]bool{}
	u.pipelines, u.repositories = nil, nil
	u.selectedRepoID = ""
	u.repositoryNotice = ""
	if u.client == nil {
		u.connect()
		return
	}
	cl := *u.client
	cl.Project = name
	u.client = &cl
	switch screen {
	case "groups":
		u.groupScreen()
	case "active-runs":
		u.activeRunsScreen()
	default:
		u.home() // Cancels reads for the previous project and advances the generation.
		u.refreshPipelines()
	}
	u.app.SetFocus(u.projectTable)
}

// Keep content as the main table so header and command-bar navigation enter it.
func (u *terminalUI) mountProjectContent(widget *tview.Table, title string, rebuild bool) {
	if rebuild {
		u.newProjectPane(widget, nil)
	} else {
		u.projectMain, u.projectBelow = widget, widget
	}
	widget.SetBorder(true).SetTitle(" " + tview.Escape(title) + " ")
	u.body.Clear()
	u.body.AddItem(tview.NewFlex().AddItem(u.projectTable, 28, 0, false).AddItem(widget, 0, 1, true), 0, 1, true)
}

func (u *terminalUI) refreshProjectList() {
	if u.client == nil {
		return
	}
	cl := u.client
	u.asyncRead(func(ctx context.Context) (func(), error) {
		projects, err := cl.Projects(ctx)
		return func() {
			u.projects, u.projectsOrg = projects, cl.Org
			u.renderProjects()
		}, err
	})
}
