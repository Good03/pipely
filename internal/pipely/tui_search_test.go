package pipely

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestSearchFocusedPane(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		for _, target := range []string{"Projects", "Repositories", "Pipelines"} {
			t.Run(fmt.Sprintf("%s/%dx%d", target, size[0], size[1]), func(t *testing.T) {
				u, screen := simulationUI(t)
				screen.SetSize(size[0], size[1])
				u.projectsOrg = u.config.Org
				u.projects = []Project{{Id: "orders", Name: "Orders"}, {Id: "payments", Name: "Payments " + strings.Repeat("long name ", 20)}}
				u.repositories = []Repo{{Id: "orders", Name: "Orders"}, {Id: "payments", Name: "Payments"}}
				u.pipelines = []Pipeline{{Id: 1, Name: "Orders build"}, {Id: 2, Name: "Payments build"}}
				u.home()
				var pane *tview.Table
				column := 0
				switch target {
				case "Projects":
					pane = u.projectTable
				case "Repositories":
					pane = u.repoTable
				case "Pipelines":
					pane = u.pipelineTable
					column = 2
				}
				u.app.SetFocus(pane)
				originalRows := pane.GetRowCount()
				sendUI(u, tcell.KeyRune, '/')
				if u.app.GetFocus() != u.commandBar || u.searchTarget != target {
					t.Fatal("/ did not search the focused pane")
				}
				for _, r := range "pAyMeNt" {
					sendUI(u, tcell.KeyRune, r)
				}
				u.app.ForceDraw()
				if pane.GetRowCount() != 2 || !strings.Contains(strings.ToLower(pane.GetCell(1, column).Text), "payments") {
					t.Fatal("search did not filter by name")
				}
				if u.app.GetFocus() != u.commandBar {
					t.Fatal("filtering stole focus")
				}
				if target == "Projects" {
					p, ok := pane.GetCell(1, 0).GetReference().(Project)
					if !ok || p.Id != "payments" {
						t.Fatal("filtered row lost project reference")
					}
				}
				if target == "Repositories" && u.selectedRepoID != "" {
					t.Fatal("typing a repository filter changed selected repository")
				}
				sendUI(u, tcell.KeyEnter, 0)
				if u.app.GetFocus() != pane || pane.GetRowCount() != 2 {
					t.Fatal("Enter did not return to filtered results")
				}
				if u.config.Project != "services" || u.screen != "pipelines" {
					t.Fatal("search Enter activated a result")
				}
				sendUI(u, tcell.KeyRune, '/')
				if u.commandBar.GetText() != "pAyMeNt" {
					t.Fatal("/ did not resume filter")
				}
				sendUI(u, tcell.KeyEscape, 0)
				if u.app.GetFocus() != pane || pane.GetRowCount() != originalRows {
					t.Fatal("Escape did not clear search and restore pane")
				}
				sendUI(u, tcell.KeyRune, '/')
				for _, r := range "no match" {
					sendUI(u, tcell.KeyRune, r)
				}
				u.app.ForceDraw()
				if pane.GetRowCount() != 1 {
					t.Fatal("no-match search retained rows")
				}
				sendUI(u, tcell.KeyDown, 0)
				if u.app.GetFocus() != pane {
					t.Fatal("Down did not return to empty results")
				}
				sendUI(u, tcell.KeyRune, '/')
				sendUI(u, tcell.KeyEscape, 0)
				if pane.GetRowCount() != originalRows {
					t.Fatal("clearing empty search did not restore rows")
				}
			})
		}
	}
}

func TestSearchDoesNotExecuteCommands(t *testing.T) {
	u, _ := simulationUI(t)
	u.pipelines = []Pipeline{{Id: 1, Name: "exit pipeline"}, {Id: 2, Name: "other"}}
	u.home()
	sendUI(u, tcell.KeyRune, '/')
	for _, r := range "exit" {
		sendUI(u, tcell.KeyRune, r)
	}
	sendUI(u, tcell.KeyEnter, 0)
	if u.ctx.Err() != nil || u.pipelineTable.GetRowCount() != 2 || u.app.GetFocus() != u.pipelineTable {
		t.Fatal("search executed a command")
	}
	sendUI(u, tcell.KeyRune, ':')
	if u.searchTarget != "" || u.app.GetFocus() != u.commandBar {
		t.Fatal(": did not restore command mode")
	}
	for _, r := range "exit" {
		sendUI(u, tcell.KeyRune, r)
	}
	sendUI(u, tcell.KeyEnter, 0)
	if u.ctx.Err() == nil {
		t.Fatal("command mode no longer executes exit")
	}
}

func TestSearchProjectPaneOnOtherTabs(t *testing.T) {
	for _, name := range []string{"active-runs", "groups"} {
		t.Run(name, func(t *testing.T) {
			u, _ := simulationUI(t)
			u.projectsOrg = u.config.Org
			u.projects = []Project{{Name: "Orders"}, {Name: "Payments"}}
			if name == "active-runs" {
				u.activeRunsScreen()
			} else {
				u.groupScreen()
			}
			u.app.SetFocus(u.projectTable)
			sendUI(u, tcell.KeyRune, '/')
			for _, r := range "order" {
				sendUI(u, tcell.KeyRune, r)
			}
			if u.projectTable.GetRowCount() != 2 {
				t.Fatal("project filter did not apply")
			}
			sendUI(u, tcell.KeyEscape, 0)
			if u.screen != name || u.app.GetFocus() != u.projectTable || u.projectTable.GetRowCount() != 4 {
				t.Fatal("Escape left the current tab")
			}
		})
	}
}

func TestRepositoryAndPipelineSearchCombine(t *testing.T) {
	u, _ := simulationUI(t)
	u.repositories = []Repo{{Id: "orders", Name: "Orders"}, {Id: "payments", Name: "Payments"}}
	u.pipelines = []Pipeline{
		{Id: 1, Name: "Orders build", Repository: &Repo{Id: "orders"}},
		{Id: 2, Name: "Payments build", Repository: &Repo{Id: "payments"}},
		{Id: 3, Name: "Payments test", Repository: &Repo{Id: "payments"}},
	}
	u.home()
	sendUI(u, tcell.KeyLeft, 0)
	sendUI(u, tcell.KeyRune, '/')
	for _, r := range "payment" {
		sendUI(u, tcell.KeyRune, r)
	}
	sendUI(u, tcell.KeyEnter, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if u.selectedRepoID != "payments" {
		t.Fatal("filtered repository activated the wrong ID")
	}
	sendUI(u, tcell.KeyRight, 0)
	sendUI(u, tcell.KeyRune, '/')
	for _, r := range "test" {
		sendUI(u, tcell.KeyRune, r)
	}
	sendUI(u, tcell.KeyEnter, 0)
	if u.pipelineTable.GetRowCount() != 2 || u.pipelineTable.GetCell(1, 2).GetReference().(Pipeline).Id != 3 {
		t.Fatal("repository and pipeline search did not combine")
	}
	u.renderRepositories()
	u.renderPipelines()
	if u.repoTable.GetRowCount() != 2 || u.pipelineTable.GetRowCount() != 2 {
		t.Fatal("refresh lost filters")
	}
	sendUI(u, tcell.KeyRune, '/')
	sendUI(u, tcell.KeyEscape, 0)
	if u.pipelineTable.GetRowCount() != 3 {
		t.Fatal("clearing pipeline search lost repository selection")
	}
}
