package pipely

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestGroupDialogShowsSavedOptionsAndExplainsDefaults(t *testing.T) {
	u, _ := simulationUI(t)
	u.pipelines = []Pipeline{{Id: 1, Name: "Orders"}, {Id: 2, Name: "Payments"}}
	g := Group{Name: "services", Project: "services", Members: []RunOptions{
		{PipelineID: 1}, {PipelineID: 2, Branch: "refs/heads/release", Stages: []string{"Test", "Prod"}},
	}}
	u.groupDetail(g)
	table := u.content.(*tview.Table)
	if table.GetCell(1, 0).Text != "Orders" || table.GetCell(1, 1).Text != "Azure default branch" || table.GetCell(1, 2).Text != "All stages" {
		t.Fatal("defaults were blank or pipeline name was missing")
	}
	if table.GetCell(2, 0).Text != "Payments" || table.GetCell(2, 1).Text != "release" || table.GetCell(2, 2).Text != "Test, Prod" {
		t.Fatal("saved options were not displayed")
	}
	sendUI(u, tcell.KeyRune, 'd')
	if front, _ := u.pages.GetFrontPage(); front != "confirm" {
		t.Fatal("delete bypassed confirmation")
	}
	sendUI(u, tcell.KeyEscape, 0)
	if u.screen != "group-detail" || u.app.GetFocus() != table {
		t.Fatal("closing confirmation did not return to group")
	}
}

func TestGroupDialogResolvesDefaultBranchesWithReadsOnly(t *testing.T) {
	u, _ := simulationUI(t)
	u.client = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("opening group made a write request: %s", r.Method)
			w.WriteHeader(400)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/pipelines"):
			fmt.Fprint(w, `{"value":[{"id":1,"name":"Orders"}]}`)
		case strings.HasSuffix(r.URL.Path, "/pipelines/1"):
			fmt.Fprint(w, `{"configuration":{"path":"azure-pipelines.yml","repository":{"id":"repo","type":"azureReposGit"}}}`)
		case strings.HasSuffix(r.URL.Path, "/refs"):
			fmt.Fprint(w, `{"value":[{"name":"refs/heads/release"},{"name":"refs/heads/main"},{"name":"refs/heads/release"}]}`)
		case strings.HasSuffix(r.URL.Path, "/items"):
			http.NotFound(w, r)
		default:
			fmt.Fprint(w, `{"defaultBranch":"refs/heads/main"}`)
		}
	})
	u.groupDetail(Group{Name: "services", Org: u.config.Org, Project: "services", Members: []RunOptions{{PipelineID: 1}}})
	loaded := make(chan struct{})
	var once sync.Once
	u.app.SetAfterDrawFunc(func(tcell.Screen) {
		table := u.content.(*tview.Table)
		panel := u.body.GetItem(0).(*centeredDialog).widget.(*dialogActions)
		if !u.busy && panel.header.GetOptionCount() == 3 && table.GetCell(1, 0).Text == "Orders" && table.GetCell(1, 1).Text == "main (default)" {
			once.Do(func() { close(loaded) })
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
	case <-loaded:
	case <-time.After(3 * time.Second):
		t.Fatal("group display metadata did not resolve")
	}
	u.path = filepath.Join(t.TempDir(), "config.json")
	u.app.QueueUpdateDraw(func() {
		sendUI(u, tcell.KeyUp, 0)
		sendUI(u, tcell.KeyEnter, 0)
		sendUI(u, tcell.KeyEnd, 0)
		sendUI(u, tcell.KeyEnter, 0)
		if len(u.config.Groups) != 1 || u.config.Groups[0].Branch != "release" {
			t.Error("could not select discovered repository branch")
		}
	})
}

func TestEditedMemberSavesToOriginalGroup(t *testing.T) {
	u, _ := simulationUI(t)
	u.path = filepath.Join(t.TempDir(), "config.json")
	u.config.Groups = []Group{{Name: "services", Org: u.config.Org, Project: u.config.Project, Members: []RunOptions{{PipelineID: 1}, {PipelineID: 2}}}}
	u.editingGroupName = "services"
	u.saveMemberForm(RunOptions{PipelineID: 1, Branch: "release", Stages: []string{"Build", "Prod"}})
	if u.screen != "group-detail" {
		t.Fatal("saving member did not return to original group")
	}
	c, err := LoadConfig(u.path)
	if err != nil || len(c.Groups) != 1 || len(c.Groups[0].Members) != 2 || c.Groups[0].Members[0].Branch != "release" || len(c.Groups[0].Members[0].Stages) != 2 {
		t.Fatalf("member edits were not saved: %+v, %v", c.Groups, err)
	}
	table := u.content.(*tview.Table)
	if table.GetCell(1, 1).Text != "release" || table.GetCell(1, 2).Text != "Build, Prod" {
		t.Fatal("updated settings were not visible")
	}
	u.editingGroupName = "services"
	u.renderRunForm(Pipeline{Id: 1}, RunOptions{}, Metadata{Branch: "main", Branches: []string{"main"}})
	sendUI(u, tcell.KeyEscape, 0)
	if u.screen != "group-detail" {
		t.Fatal("Escape from member settings did not return to group")
	}
}

func TestEnterWhileEditingGroupMemberSavesWithoutLaunching(t *testing.T) {
	for _, implicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("implicit=%t", implicit), func(t *testing.T) {
			u, _ := simulationUI(t)
			u.path = filepath.Join(t.TempDir(), "config.json")
			u.config.Groups = []Group{{Name: "services", Org: u.config.Org, Project: u.config.Project, Members: []RunOptions{{PipelineID: 1}, {PipelineID: 2}}}}
			u.editingGroupName = "services"
			var stages []Stage
			if !implicit {
				var err error
				stages, err = ParseStages("stages:\n- stage: Build\n- stage: Test\n- stage: Prod\n")
				if err != nil {
					t.Fatal(err)
				}
			}
			u.stageScreen(Pipeline{Id: 1}, Plan{Options: RunOptions{PipelineID: 1, Branch: "release", Parameters: map[string]any{"tests": true}}, Stages: stages}, nil)
			if !implicit {
				table := u.content.(*tview.Table)
				for row := 1; row <= 3; row++ {
					if !strings.Contains(tview.Unescape(table.GetCell(row, 0).Text), "[x]") {
						t.Fatal("editing changed the group's all-stages default")
					}
				}
			}
			sendUI(u, tcell.KeyEnter, 0)
			if u.screen != "group-detail" || u.busy {
				t.Fatal("Enter attempted a launch instead of saving locally")
			}
			if front, _ := u.pages.GetFrontPage(); front != "main" {
				t.Fatal("member editing opened a launch confirmation")
			}
			c, err := LoadConfig(u.path)
			if err != nil {
				t.Fatal(err)
			}
			member := c.Groups[0].Members[0]
			if member.Branch != "release" || member.Parameters["tests"] != true || len(c.Groups[0].Members) != 2 {
				t.Fatalf("member changes not preserved: %+v", member)
			}
			if len(member.Stages) != len(stages) {
				t.Fatalf("stages not saved: %v", member.Stages)
			}
		})
	}
}

func TestGroupNameNavigationSkipsProjectLabelAndSaves(t *testing.T) {
	for _, key := range []tcell.Key{tcell.KeyDown, tcell.KeyTab, tcell.KeyEnter} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			u, _ := simulationUI(t)
			u.path = filepath.Join(t.TempDir(), "config.json")
			u.pipelines = []Pipeline{{Id: 1, Name: "Orders"}, {Id: 2, Name: "Payments"}}
			u.selected[1], u.selected[2] = true, true
			u.saveSelectedGroup()
			for _, letter := range "services" {
				sendUI(u, tcell.KeyRune, letter)
			}
			u.app.ForceDraw()
			sendUI(u, key, 0)
			if u.app.GetFocus() != u.form.GetButton(0) {
				t.Fatal("navigation did not skip the project label and focus Save")
			}
			sendUI(u, tcell.KeyUp, 0)
			if u.app.GetFocus() != u.form.GetFormItem(0) {
				t.Fatal("Up did not return directly to the group name")
			}
			sendUI(u, tcell.KeyDown, 0)
			sendUI(u, tcell.KeyEnter, 0)
			if u.screen != "groups" {
				t.Fatal("Save did not return to groups")
			}
			c, err := LoadConfig(u.path)
			if err != nil || len(c.Groups) != 1 || c.Groups[0].Name != "services" || len(c.Groups[0].Members) != 2 {
				t.Fatalf("group was not saved correctly: %+v, %v", c.Groups, err)
			}
		})
	}
}

func TestConfiguredGroupSurvivesRestartAndMembershipUpdate(t *testing.T) {
	u, _ := simulationUI(t)
	u.path = filepath.Join(t.TempDir(), "config.json")
	u.config.Groups = []Group{{Name: "services", Org: u.config.Org, Project: u.config.Project, Members: []RunOptions{{PipelineID: 1}, {PipelineID: 2}}}}
	opts := RunOptions{PipelineID: 1, Branch: "release", Stages: []string{"Build", "Prod"}, Parameters: map[string]any{"tests": true, "environment": "prod"}}
	u.editingGroupName = "services"
	u.saveMemberForm(opts)

	// Start a fresh UI from exactly the settings entrypoint used by the CLI.
	loaded, err := ReadSettings(u.path)
	if err != nil {
		t.Fatal(err)
	}
	restarted, _ := simulationUI(t)
	restarted.config, restarted.path = loaded, u.path
	restarted.groupScreen()
	if len(restarted.groups) != 1 || !reflect.DeepEqual(restarted.groups[0].Members[0], opts) {
		t.Fatalf("relaunch lost configured options: %+v", restarted.groups)
	}
	sendUI(restarted, tcell.KeyEnter, 0)
	table := restarted.content.(*tview.Table)
	if table.GetCell(1, 1).Text != "release" || table.GetCell(1, 2).Text != "Build, Prod" {
		t.Fatal("reloaded group view did not show saved settings")
	}

	// Reselecting an existing pipeline and adding a new one keeps the former's
	// configuration while applying the requested membership change.
	restarted.groupNameForm([]RunOptions{{PipelineID: 1}, {PipelineID: 3}}, false)
	restarted.form.GetFormItem(0).(*tview.InputField).SetText("services")
	sendUI(restarted, tcell.KeyDown, 0)
	sendUI(restarted, tcell.KeyEnter, 0)
	reloaded, err := ReadSettings(u.path)
	if err != nil {
		t.Fatal(err)
	}
	g, err := FindGroup(reloaded, "services")
	if err != nil || len(g.Members) != 2 || !reflect.DeepEqual(g.Members[0], opts) || g.Members[1].PipelineID != 3 {
		t.Fatalf("updating group membership reset options: %+v, %v", g, err)
	}
}

func TestFailedGroupSaveDoesNotChangeRememberedOptions(t *testing.T) {
	u, _ := simulationUI(t)
	root := t.TempDir()
	u.path = filepath.Join(root, "config.json")
	member := RunOptions{PipelineID: 1, Branch: "main", Stages: []string{"Build"}}
	g := Group{Name: "services", Org: u.config.Org, Project: u.config.Project, Members: []RunOptions{member}}
	if err := u.persistGroup(g); err != nil {
		t.Fatal(err)
	}
	savedPath := u.path
	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	u.path = filepath.Join(blocker, "config.json")
	u.editingGroupName = "services"
	u.saveMemberForm(RunOptions{PipelineID: 1, Branch: "release", Stages: []string{"Prod"}})
	if u.lastError == "" || !reflect.DeepEqual(u.config.Groups[0].Members[0], member) {
		t.Fatal("failed save was not reported or changed the in-memory configuration")
	}
	c, err := ReadSettings(savedPath)
	if err != nil || !reflect.DeepEqual(c.Groups[0].Members[0], member) {
		t.Fatal("failed save changed the persisted configuration")
	}
}

func TestGroupDialogActionButtonsAndArrowNavigation(t *testing.T) {
	u, screen := simulationUI(t)
	u.groupDetail(Group{Name: "services", Project: "services", Members: []RunOptions{{PipelineID: 1}, {PipelineID: 2}}})
	u.app.ForceDraw()
	table := u.content.(*tview.Table)
	panel := u.body.GetItem(0).(*centeredDialog).widget.(*dialogActions)
	if table.GetRowCount() != 3 || panel.actions.GetButtonCount() != 2 {
		t.Fatal("group actions were not moved out of the member table")
	}
	for i := 0; i < panel.actions.GetButtonCount(); i++ {
		x, y, width, _ := panel.actions.GetButton(i).GetRect()
		for col := x; col < x+width; col++ {
			_, _, style, _ := screen.GetContent(col, y)
			_, background, _ := style.Decompose()
			if background != dialogBackground {
				t.Fatal("inactive action button created a separate dark block")
			}
		}
	}
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyDown, 0)
	if u.app.GetFocus() != panel.actions.GetButton(0) {
		t.Fatal("Down from the last pipeline did not focus Run group")
	}
	u.app.ForceDraw()
	if purpleSelectionCells(u.terminalScreen, table) != 0 {
		t.Fatal("member table retained a focus highlight while buttons were active")
	}
	x, y, width, height := table.GetRect()
	for row := y; row < y+height; row++ {
		for col := x; col < x+width; col++ {
			_, _, style, _ := screen.GetContent(col, row)
			_, background, _ := style.Decompose()
			if background != dialogBackground {
				t.Fatal("inactive member table created a separate dark block")
			}
		}
	}
	sendUI(u, tcell.KeyRight, 0)
	if u.app.GetFocus() != panel.actions.GetButton(1) {
		t.Fatal("Right did not focus Delete")
	}
	sendUI(u, tcell.KeyEnter, 0)
	if front, _ := u.pages.GetFrontPage(); front != "confirm" {
		t.Fatal("Delete button did not request confirmation")
	}
	sendUI(u, tcell.KeyEscape, 0)
	if u.app.GetFocus() != panel.actions.GetButton(1) {
		t.Fatal("closing confirmation did not restore the button focus")
	}
	sendUI(u, tcell.KeyLeft, 0)
	sendUI(u, tcell.KeyUp, 0)
	if u.app.GetFocus() != table {
		t.Fatal("Up did not return to group members")
	}
	sendUI(u, tcell.KeyEscape, 0)
	if u.screen != "groups" {
		t.Fatal("Escape did not close the group dialog")
	}
}

func TestGroupBranchSavedAndAppliedThroughRootLayout(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			u, screen := simulationUI(t)
			screen.SetSize(size[0], size[1])
			u.path = filepath.Join(t.TempDir(), "config.json")
			g := Group{Name: "services", Org: u.config.Org, Project: u.config.Project, Members: []RunOptions{
				{PipelineID: 1, Branch: "main", Stages: []string{"Build"}, Parameters: map[string]any{"tests": true}},
				{PipelineID: 2, Branch: "old"},
			}}
			u.config.Groups = []Group{g}
			u.groupDetail(g)
			u.app.ForceDraw()
			sendUI(u, tcell.KeyUp, 0)
			field, ok := u.app.GetFocus().(*tview.DropDown)
			if !ok {
				t.Fatal("Up from first member did not focus branch")
			}
			field.AddOption("feature/group-release", nil)
			sendUI(u, tcell.KeyEnter, 0)
			if !u.dropdownOpen() {
				t.Fatal("Enter did not open branch dropdown")
			}
			sendUI(u, tcell.KeyEnd, 0)
			sendUI(u, tcell.KeyEscape, 0)
			if u.dropdownOpen() || u.screen != "group-detail" {
				t.Fatal("Escape did not close only the dropdown")
			}
			sendUI(u, tcell.KeyEnter, 0)
			sendUI(u, tcell.KeyEnd, 0)
			u.app.ForceDraw()
			x, y, width, height := field.GetRect()
			if x < 0 || y < 0 || x+width > size[0] || y+height > size[1] {
				t.Fatal("branch field outside terminal")
			}
			sendUI(u, tcell.KeyEnter, 0)
			table := u.content.(*tview.Table)
			if u.app.GetFocus() != table {
				t.Fatal("Enter did not return to members")
			}
			for row := 1; row <= 2; row++ {
				if table.GetCell(row, 1).Text != "feature/group-release" {
					t.Fatal("shared branch not shown for every member")
				}
			}
			loaded, err := ReadSettings(u.path)
			if err != nil || loaded.Groups[0].Branch != "feature/group-release" || !reflect.DeepEqual(loaded.Groups[0].Members, g.Members) {
				t.Fatalf("shared branch did not persist independently of member options: %+v, %v", loaded.Groups, err)
			}
			// Member forms expose branch as information, leaving parameters and stages editable.
			u.editingGroupName = g.Name
			u.renderRunForm(Pipeline{Id: 1}, RunOptions{}, Metadata{Branch: loaded.Groups[0].Branch, Branches: []string{"main", loaded.Groups[0].Branch}})
			if _, ok := u.form.GetFormItem(0).(*tview.TextView); !ok {
				t.Fatal("member still has an independent branch selector")
			}
			sendUI(u, tcell.KeyEscape, 0)
			if u.screen != "group-detail" || u.content.(*tview.Table).GetCell(2, 1).Text != loaded.Groups[0].Branch {
				t.Fatal("returning from member lost shared branch")
			}
			// Updating membership retains the branch after restart.
			u.config = loaded
			u.groupNameForm([]RunOptions{{PipelineID: 1}, {PipelineID: 3}}, false)
			u.form.GetFormItem(0).(*tview.InputField).SetText(g.Name)
			sendUI(u, tcell.KeyDown, 0)
			sendUI(u, tcell.KeyEnter, 0)
			updated, err := ReadSettings(u.path)
			if err != nil || updated.Groups[0].Branch != loaded.Groups[0].Branch {
				t.Fatal("membership update lost shared branch")
			}
		})
	}
}

func TestFailedSharedBranchSaveKeepsGroupAndFocus(t *testing.T) {
	u, _ := simulationUI(t)
	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	u.path = filepath.Join(blocker, "config.json")
	g := Group{Name: "services", Org: u.config.Org, Project: u.config.Project, Branch: "main", Members: []RunOptions{{PipelineID: 1}}}
	u.config.Groups = []Group{g}
	u.groupDetail(g)
	sendUI(u, tcell.KeyUp, 0)
	field := u.app.GetFocus().(*tview.DropDown)
	field.AddOption("release", nil)
	sendUI(u, tcell.KeyEnter, 0)
	sendUI(u, tcell.KeyEnd, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if u.lastError == "" || !reflect.DeepEqual(u.config.Groups[0], g) {
		t.Fatal("failed branch save changed remembered options")
	}
	sendUI(u, tcell.KeyEscape, 0) // Close the error and allow correction in place.
	if u.app.GetFocus() != field || u.screen != "group-detail" {
		t.Fatal("failed branch save lost branch field focus")
	}
}
