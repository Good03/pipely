package pipely

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestProjectSidebarDiscoveryAndSwitch(t *testing.T) {
	u, _ := simulationUI(t)
	u.path = filepath.Join(t.TempDir(), "config.json")
	u.client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_apis/projects"):
			fmt.Fprint(w, `{"value":[{"id":"one","name":"services"},{"id":"two","name":"payments"}]}`)
		case strings.HasSuffix(r.URL.Path, "/_apis/build/definitions"):
			if strings.Contains(r.URL.Path, "/payments/") {
				fmt.Fprint(w, `{"value":[{"id":1,"name":"payments build","repository":{"id":"payments","name":"payments"}}]}`)
			} else {
				fmt.Fprint(w, `{"value":[{"id":1,"name":"services build","repository":{"id":"services","name":"services"}}]}`)
			}
		case strings.HasSuffix(r.URL.Path, "/_apis/git/repositories"):
			fmt.Fprint(w, `{"value":[]}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	u.client.Org, u.client.Project = "example", "services"
	u.home()
	first, second := make(chan struct{}), make(chan struct{})
	var firstOnce, secondOnce sync.Once
	u.app.SetAfterDrawFunc(func(tcell.Screen) {
		if !u.busy && len(u.projects) == 2 && len(u.pipelines) == 1 {
			if u.config.Project == "services" {
				firstOnce.Do(func() { close(first) })
			} else if u.config.Project == "payments" {
				secondOnce.Do(func() { close(second) })
			}
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
			t.Error("TUI event loop did not stop")
		}
	})
	u.app.QueueUpdateDraw(u.refreshPipelines)
	select {
	case <-first:
	case <-time.After(3 * time.Second):
		t.Fatal("project discovery did not complete")
	}
	u.app.QueueUpdateDraw(func() {
		if u.projectTable.GetRowCount() != 3 {
			t.Error("sidebar omitted an available project")
		}
		u.selected[1] = true
		u.selectedRepoID = "services"
		sendUI(u, tcell.KeyLeft, 0)
		sendUI(u, tcell.KeyUp, 0)
		if u.app.GetFocus() != u.projectTable {
			t.Error("Up from repositories did not enter projects")
		}
		// The current project sorts second. Move to payments and activate it.
		sendUI(u, tcell.KeyUp, 0)
		sendUI(u, tcell.KeyEnter, 0)
		if len(u.selected) != 0 || u.selectedRepoID != "" || len(u.pipelines) != 0 {
			t.Error("project switch retained stale data or group marks")
		}
	})
	select {
	case <-second:
	case <-time.After(3 * time.Second):
		t.Fatal("new project's catalog did not load")
	}
	u.app.QueueUpdateDraw(func() {
		if u.client.Project != "payments" || u.pipelineTable.GetCell(1, 2).Text != "payments build" {
			t.Error("new project displayed the wrong pipelines")
		}
		if u.projectTable.GetRowCount() != 3 || u.app.GetFocus() != u.projectTable {
			t.Error("switch lost the project list or stole focus")
		}
	})
	c, err := LoadConfig(u.path)
	if err != nil || c.Project != "payments" {
		t.Fatalf("project selection not saved: %v, %v", c.Project, err)
	}
}

func TestProjectSidebarDoesNotShowAnotherOrganization(t *testing.T) {
	u, _ := simulationUI(t)
	u.projectsOrg = "other"
	u.projects = []Project{{Name: "private-project"}}
	u.home()
	if u.projectTable.GetRowCount() != 2 || strings.Contains(u.projectTable.GetCell(1, 0).Text, "private-project") {
		t.Fatal("sidebar reused another organization's projects")
	}
}

func TestProjectFiltersStayOnGroupsAndActiveTabs(t *testing.T) {
	for _, tab := range []string{"groups", "active-runs"} {
		t.Run(tab, func(t *testing.T) {
			u, _ := simulationUI(t)
			u.path = filepath.Join(t.TempDir(), "config.json")
			u.config.Groups = []Group{
				{Name: "deploy", Org: "example", Project: "services", Members: []RunOptions{{PipelineID: 1}}},
				{Name: "deploy", Org: "example", Project: "payments", Members: []RunOptions{{PipelineID: 2}}},
				{Name: "hidden", Org: "other", Project: "payments", Members: []RunOptions{{PipelineID: 3}}},
			}
			u.client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/_apis/projects"):
					fmt.Fprint(w, `{"value":[{"name":"services"},{"name":"payments"}]}`)
				case strings.HasSuffix(r.URL.Path, "/_apis/build/builds"):
					name := "services"
					if strings.Contains(r.URL.Path, "/payments/") {
						name = "payments"
					}
					fmt.Fprintf(w, `{"value":[{"id":123,"status":"inProgress","definition":{"id":1,"name":"%s build"}}]}`, name)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			u.client.Org, u.client.Project = "example", "services"
			if tab == "groups" {
				u.groupScreen()
			} else {
				u.activeRunsScreen()
			}
			first, second := make(chan struct{}), make(chan struct{})
			var firstOnce, secondOnce sync.Once
			u.app.SetAfterDrawFunc(func(tcell.Screen) {
				if !u.busy && len(u.projects) == 2 {
					if u.config.Project == "services" {
						firstOnce.Do(func() { close(first) })
					} else if u.config.Project == "payments" {
						secondOnce.Do(func() { close(second) })
					}
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
					t.Error("TUI event loop did not stop")
				}
			})
			select {
			case <-first:
			case <-time.After(3 * time.Second):
				t.Fatal("initial project filter did not load")
			}
			u.app.QueueUpdateDraw(func() {
				sendUI(u, tcell.KeyLeft, 0)
				if u.app.GetFocus() != u.projectTable {
					t.Error("Left did not focus projects")
				}
				sendUI(u, tcell.KeyUp, 0)
				sendUI(u, tcell.KeyEnter, 0)
			})
			select {
			case <-second:
			case <-time.After(3 * time.Second):
				t.Fatal("selected project did not load")
			}
			u.app.QueueUpdateDraw(func() {
				if u.screen != tab || u.client.Project != "payments" || u.app.GetFocus() != u.projectTable {
					t.Error("project selection changed tabs, used the wrong client, or stole focus")
				}
				if tab == "groups" {
					if len(u.groups) != 1 || u.groups[0].Project != "payments" || u.groups[0].Members[0].PipelineID != 2 {
						t.Errorf("groups leaked across projects or organizations: %+v", u.groups)
					}
				} else {
					if u.projectMain.GetCell(1, 1).Text != "payments build" {
						t.Error("active runs showed the previous project")
					}
					sidebar := u.projectTable
					u.renderActiveRuns([]Build{{ID: 456, Definition: Pipeline{Name: "refreshed"}}})
					if u.app.GetFocus() != sidebar || u.projectTable != sidebar {
						t.Error("live refresh replaced the sidebar or stole focus")
					}
					sendUI(u, tcell.KeyRight, 0)
					if u.app.GetFocus() != u.content || u.projectMain.GetCell(1, 1).Text != "refreshed" {
						t.Error("sidebar navigation targeted an obsolete run table")
					}
				}
			})
		})
	}
}

func TestNewGroupUsesSelectedProject(t *testing.T) {
	u, _ := simulationUI(t)
	u.path = filepath.Join(t.TempDir(), "config.json")
	u.config.Project = "payments"
	u.groupNameForm([]RunOptions{{PipelineID: 42}}, false)
	u.form.GetFormItem(0).(*tview.InputField).SetText("deploy")
	u.form.SetFocus(u.form.GetFormItemCount())
	u.app.SetFocus(u.form)
	sendUI(u, tcell.KeyEnter, 0)
	g, err := FindGroup(u.config, "deploy")
	if err != nil || g.Project != "payments" || g.Org != "example" {
		t.Fatalf("new group has incorrect assignment: %+v, %v", g, err)
	}
	c, err := LoadConfig(u.path)
	if err != nil || len(c.Groups) != 1 || c.Groups[0].Project != "payments" {
		t.Fatalf("project assignment was not persisted: %+v, %v", c.Groups, err)
	}
}
