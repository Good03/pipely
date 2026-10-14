package pipely

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestFormsAreCenteredAndResize(t *testing.T) {
	for _, name := range []string{"organization", "authentication", "group", "run"} {
		t.Run(name, func(t *testing.T) {
			u, screen := simulationUI(t)
			switch name {
			case "organization":
				u.contextForm()
			case "authentication":
				u.authForm()
			case "group":
				u.groupNameForm([]RunOptions{{PipelineID: 1}}, false)
			case "run":
				u.renderRunForm(Pipeline{Id: 1, Name: "orders"}, RunOptions{}, Metadata{Branch: "main", Branches: []string{"main"}, Parameters: []Parameter{{Name: "tests", Type: "boolean", Default: true}}})
			}
			for _, size := range [][2]int{{120, 35}, {80, 24}} {
				screen.SetSize(size[0], size[1])
				u.app.ForceDraw()
				x, y, width, height := u.form.GetRect()
				bx, by, bw, bh := u.body.GetInnerRect()
				if width >= bw || height >= bh || x < bx || y < by || x+width > bx+bw || y+height > by+bh {
					t.Fatalf("form is not contained in a dialog: form=%d,%d %dx%d, body=%d,%d %dx%d", x, y, width, height, bx, by, bw, bh)
				}
				if abs((x-bx)-(bx+bw-x-width)) > 1 || abs((y-by)-(by+bh-y-height)) > 1 {
					t.Fatal("dialog is not centered after terminal resize")
				}
				if !u.form.HasFocus() {
					t.Fatal("dialog lost keyboard focus")
				}
			}
			before := u.screen
			sendUI(u, tcell.KeyF4, 0)
			if u.app.GetFocus() == u.tabs {
				t.Fatal("dialog allowed focus to escape into header tabs")
			}
			sendUI(u, tcell.KeyF1, 0)
			sendUI(u, tcell.KeyEscape, 0)
			if u.screen != before || !u.form.HasFocus() {
				t.Fatal("closing help did not restore the dialog")
			}
			sendUI(u, tcell.KeyEscape, 0)
			if u.screen == before {
				t.Fatal("Escape did not leave the dialog")
			}
		})
	}
}

func TestTallRunDialogKeepsLastActionVisible(t *testing.T) {
	u, screen := simulationUI(t)
	screen.SetSize(80, 24)
	params := []Parameter{}
	for i := 0; i < 20; i++ {
		params = append(params, Parameter{Name: fmt.Sprintf("option%d", i), Type: "boolean"})
	}
	u.renderRunForm(Pipeline{Id: 1}, RunOptions{}, Metadata{Branch: "main", Branches: []string{"main"}, Parameters: params})
	u.form.SetFocus(u.form.GetFormItemCount())
	u.app.SetFocus(u.form)
	u.app.ForceDraw()
	x, y, width, height := u.form.GetInnerRect()
	button := u.form.GetButton(0)
	bx, by, bw, bh := button.GetRect()
	if !button.HasFocus() || bx < x || by < y || bx+bw > x+width || by+bh > y+height {
		t.Fatal("tall dialog did not scroll to its focused action")
	}
}

func TestCenteredDialogForwardsInputThroughRoot(t *testing.T) {
	u, screen := simulationUI(t)
	f := tview.NewForm()
	branch := "main"
	text := ""
	selected := false
	f.AddDropDown("Branch", []string{"main", "feat/dialog-fix"}, 0, func(value string, _ int) { branch = value })
	f.AddCheckbox("Tests", false, func(value bool) { selected = value })
	f.AddInputField("Name", "", 20, nil, func(value string) { text = value })
	continued := false
	f.AddButton("Continue", func() { continued = true })
	u.show("run-form", "Configure run", f)
	u.app.ForceDraw()
	sendUI(u, tcell.KeyEnter, 0)
	u.app.ForceDraw()
	if !u.dropdownOpen() {
		t.Fatal("Enter did not open branch selection through the root layout")
	}
	cells, _, _ := screen.GetContents()
	var displayed strings.Builder
	for _, cell := range cells {
		displayed.WriteString(string(cell.Runes))
	}
	if !strings.Contains(displayed.String(), "feat/dialog-fix") {
		t.Fatal("open branch menu was not drawn")
	}
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if branch != "feat/dialog-fix" || u.dropdownOpen() {
		t.Fatal("branch choice was not applied or menu did not close")
	}
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyRune, ' ')
	if !selected {
		t.Fatal("checkbox did not receive input")
	}
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyRune, 'a')
	u.pages.PasteHandler()("bc", func(p tview.Primitive) { u.app.SetFocus(p) })
	if text != "abc" {
		t.Fatalf("typing/pasting through dialog failed: %q", text)
	}
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if !continued {
		t.Fatal("could not continue past the branch selector")
	}
}

func TestOptionPickersAreCenteredAndKeepFocus(t *testing.T) {
	for _, name := range []string{"contexts", "projects", "stages", "group-detail", "links"} {
		t.Run(name, func(t *testing.T) {
			u, screen := simulationUI(t)
			var widget tview.Primitive
			switch name {
			case "contexts":
				u.config.Contexts = []Context{{Org: "example", Project: "services"}, {Org: "example", Project: "payments"}}
				u.contextScreen()
				widget = u.body.GetItem(0).(*centeredDialog).widget
			case "projects":
				list := tview.NewList().ShowSecondaryText(false)
				list.AddItem("Services", "", 0, nil).AddItem("Payments", "", 0, nil)
				u.show("projects", "Select project", list)
				widget = list
			case "stages":
				stages, err := ParseStages("stages:\n- stage: Build\n- stage: Test\n")
				if err != nil {
					t.Fatal(err)
				}
				u.stageScreen(Pipeline{Id: 1}, Plan{Options: RunOptions{PipelineID: 1}, Stages: stages}, nil)
				widget = u.content
			case "group-detail":
				u.groupDetail(Group{Name: "services", Members: []RunOptions{{PipelineID: 1}}})
				widget = u.body.GetItem(0).(*centeredDialog).widget
			case "links":
				u.home()
				u.linkPicker("https://example.com/one https://example.com/two")
				widget = u.app.GetFocus()
			}
			for _, size := range [][2]int{{120, 35}, {80, 24}} {
				screen.SetSize(size[0], size[1])
				u.app.ForceDraw()
				x, y, width, height := widget.GetRect()
				bx, by, bw, bh := u.body.GetInnerRect()
				if name == "links" {
					bx, by, bw, bh = 0, 0, size[0], size[1]
				}
				if width >= bw || height >= bh || abs((x-bx)-(bx+bw-x-width)) > 1 || abs((y-by)-(by+bh-y-height)) > 1 {
					t.Fatal("option picker is not centered within the available space")
				}
			}
			sendUI(u, tcell.KeyDown, 0)
			if list, ok := widget.(*tview.List); ok && list.GetCurrentItem() != 1 {
				t.Fatal("root input did not reach the option list")
			}
			if table, ok := widget.(*tview.Table); ok {
				row, _ := table.GetSelection()
				if row != 2 {
					t.Fatal("root input did not reach stage selection")
				}
			}
			sendUI(u, tcell.KeyUp, 0)
			sendUI(u, tcell.KeyUp, 0)
			sendUI(u, tcell.KeyF4, 0)
			if u.app.GetFocus() == u.tabs {
				t.Fatal("option picker lost focus to the header")
			}
			if name == "stages" {
				before := u.screen
				widget.(*tview.Table).Select(2, 0)
				sendUI(u, tcell.KeyRune, ' ')
				if before != u.screen || !strings.Contains(tview.Unescape(widget.(*tview.Table).GetCell(2, 0).Text), "[x]") {
					t.Fatal("stage checkbox did not receive input")
				}
			}
			if name != "stages" { // Returning to the run form needs an Azure client.
				sendUI(u, tcell.KeyEscape, 0)
				if name == "links" {
					if front, _ := u.pages.GetFrontPage(); front != "main" {
						t.Fatal("Escape did not close link picker")
					}
				} else if u.screen == name {
					t.Fatal("Escape did not leave option picker")
				}
			}
		})
	}
}

func TestDialogInitiallySkipsNoticesAndDisabledButtons(t *testing.T) {
	u, _ := simulationUI(t)
	form := tview.NewForm().AddTextView("Options", "Loading…", 40, 1, false, false)
	form.AddButton("Unavailable", nil)
	form.GetButton(0).SetDisabled(true)
	retried := false
	form.AddButton("Retry", func() { retried = true })
	u.show("run-form", "Loading options", form)
	u.app.ForceDraw()
	if u.app.GetFocus() != form.GetButton(1) {
		t.Fatal("dialog did not initially focus the available action")
	}
	sendUI(u, tcell.KeyUp, 0)
	sendUI(u, tcell.KeyDown, 0)
	if u.app.GetFocus() != form.GetButton(1) {
		t.Fatal("navigation selected a notice or disabled action")
	}
	sendUI(u, tcell.KeyEnter, 0)
	if !retried {
		t.Fatal("the available action did not receive input")
	}
}

func TestBranchPickerHighlightsOnlySelectedOption(t *testing.T) {
	u, screen := simulationUI(t)
	u.renderRunForm(Pipeline{Id: 1}, RunOptions{}, Metadata{Branch: "main", Branches: []string{"main", "feature-one", "feature-two"}})
	u.app.ForceDraw()
	sendUI(u, tcell.KeyEnter, 0)
	check := func(selected string) {
		t.Helper()
		u.app.ForceDraw()
		width, height := screen.Size()
		counts := map[string]int{}
		for y := 0; y < height; y++ {
			var line strings.Builder
			for x := 0; x < width; x++ {
				r, _, style, _ := screen.GetContent(x, y)
				foreground, background, _ := style.Decompose()
				if foreground == tcell.ColorGreen || background == tcell.ColorGreen {
					t.Fatal("branch picker contains default green styling")
				}
				line.WriteRune(r)
			}
			for _, branch := range []string{"main", "feature-one", "feature-two"} {
				if x := strings.Index(line.String(), branch); x >= 0 {
					x = len([]rune(line.String()[:x]))
					_, _, style, _ := screen.GetContent(x, y)
					_, background, _ := style.Decompose()
					if background == purple {
						counts[branch]++
					}
				}
			}
		}
		if counts[selected] != 1 || len(counts) != 1 {
			t.Fatalf("expected only %s highlighted, got %v", selected, counts)
		}
	}
	check("main")
	sendUI(u, tcell.KeyDown, 0)
	check("feature-one")
	sendUI(u, tcell.KeyEscape, 0)
	if u.dropdownOpen() {
		t.Fatal("Escape did not close the branch picker")
	}
}

func TestMessageAndDiagnosticDialogsShareLayoutAndScroll(t *testing.T) {
	for _, name := range []string{"help", "confirm", "info"} {
		t.Run(name, func(t *testing.T) {
			u, screen := simulationUI(t)
			u.home()
			previous := u.app.GetFocus()
			text := strings.Repeat("Pipeline details and diagnostic output\n", 60)
			switch name {
			case "help":
				u.help()
			case "confirm":
				u.confirm(text, func() { t.Error("scrolling confirmed the action") })
			case "info":
				u.info("Diagnostics", text)
			}
			front, primitive := u.pages.GetFrontPage()
			dialog, ok := primitive.(*centeredDialog)
			if front != name || !ok {
				t.Fatal("overlay bypassed the shared dialog component")
			}
			var view *tview.TextView
			if panel, ok := dialog.widget.(*dialogActions); ok {
				view = panel.content.(*tview.TextView)
			} else {
				view = dialog.widget.(*tview.TextView)
			}
			for _, size := range [][2]int{{120, 35}, {80, 24}} {
				screen.SetSize(size[0], size[1])
				u.app.ForceDraw()
				x, y, width, height := dialog.widget.GetRect()
				if x < 2 || y < 1 || x+width > size[0]-2 || y+height > size[1]-1 || abs(2*x+width-size[0]) > 1 || abs(2*y+height-size[1]) > 1 {
					t.Fatal("overlay overflowed or was not centered")
				}
				_, _, style, _ := screen.GetContent(x+1, y+1)
				_, background, _ := style.Decompose()
				if background != dialogBackground {
					t.Fatalf("overlay used a different surface color: got %v at %d,%d, expected %v", background, x+1, y+1, dialogBackground)
				}
			}
			sendUI(u, tcell.KeyPgDn, 0)
			u.app.ForceDraw()
			if row, _ := view.GetScrollOffset(); row == 0 {
				t.Fatal("long overlay content could not be scrolled")
			}
			sendUI(u, tcell.KeyEscape, 0)
			if front, _ := u.pages.GetFrontPage(); front != "main" || u.app.GetFocus() != previous {
				t.Fatal("closing overlay did not restore workspace focus")
			}
		})
	}
}

func TestSharedConfirmationButtonsReceiveRootInput(t *testing.T) {
	u, _ := simulationUI(t)
	u.home()
	confirmed := false
	u.confirm("Run services?", func() { confirmed = true })
	u.app.ForceDraw()
	sendUI(u, tcell.KeyRight, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if confirmed {
		t.Fatal("Back confirmed the action")
	}
	u.confirm("Run services?", func() { confirmed = true })
	u.app.ForceDraw()
	sendUI(u, tcell.KeyEnter, 0)
	if !confirmed {
		t.Fatal("Confirm did not receive input through the dialog layout")
	}
}

func TestContextPickerSeparatesOptionsAndActions(t *testing.T) {
	u, _ := simulationUI(t)
	u.config.Contexts = []Context{{Org: "example", Project: "services"}, {Org: "example", Project: "payments"}}
	u.contextScreen()
	u.app.ForceDraw()
	list := u.content.(*tview.List)
	if list.GetItemCount() != 2 {
		t.Fatal("context list contains action or Back entries")
	}
	panel := u.body.GetItem(0).(*centeredDialog).widget.(*dialogActions)
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyDown, 0)
	if u.app.GetFocus() != panel.actions.GetButton(0) {
		t.Fatal("Down did not reach Add organization")
	}
	sendUI(u, tcell.KeyUp, 0)
	if u.app.GetFocus() != list {
		t.Fatal("Up did not return to saved contexts")
	}
	sendUI(u, tcell.KeyDown, 0)
	sendUI(u, tcell.KeyEnter, 0)
	if u.screen != "context-form" {
		t.Fatal("Add organization button did not open the form")
	}
	sendUI(u, tcell.KeyEscape, 0)
	sendUI(u, tcell.KeyEscape, 0)
	if u.screen != "pipelines" {
		t.Fatal("Escape did not return to the workspace")
	}
	u.config.Contexts = nil
	u.contextScreen()
	u.app.ForceDraw()
	if _, ok := u.app.GetFocus().(*tview.Button); !ok {
		t.Fatal("empty context picker did not focus its available action")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
