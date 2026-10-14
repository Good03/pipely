package pipely

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *terminalUI) newCommandBar() *tview.InputField {
	bar := tview.NewInputField().SetLabel(" : Command ").SetPlaceholder("Type to filter pipelines; Enter executes exit, help, active, groups…")
	bar.SetLabelColor(purple)
	bar.SetChangedFunc(func(text string) {
		if u.updatingCommandBar {
			return
		}
		if u.searchTarget != "" {
			u.applySearch(text)
			return
		}
		u.pipelineFilter = text
		if u.screen == "pipelines" && u.renderPipelines != nil {
			u.renderPipelines()
		}
	})
	bar.SetDoneFunc(func(key tcell.Key) {
		if u.searchTarget != "" {
			u.leaveSearch(key == tcell.KeyEscape)
			return
		}
		if key == tcell.KeyEnter && u.executeCommand(bar.GetText()) {
			return
		}
		if key == tcell.KeyEscape || key == tcell.KeyEnter || key == tcell.KeyTab {
			if u.content != nil {
				u.app.SetFocus(u.content)
			}
		}
	})
	bar.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyDown {
			if u.searchTarget != "" {
				u.leaveSearch(false)
				return nil
			}
			if u.content != nil {
				u.app.SetFocus(u.content)
			}
			return nil
		}
		if e.Key() == tcell.KeyUp {
			u.app.SetFocus(u.tabs)
			return nil
		}
		return e
	})
	return bar
}
func (u *terminalUI) executeCommand(text string) bool {
	command := strings.ToLower(strings.TrimSpace(text))
	command = strings.TrimPrefix(command, ":")
	var action func()
	switch command {
	case "exit", "quit", "q":
		action = func() { u.cancel(); u.app.Stop() }
	case "help", "?":
		action = u.help
	case "pipelines":
		action = u.home
	case "active", "runs":
		action = u.activeRunsScreen
	case "groups":
		action = u.groupScreen
	case "context", "org":
		action = u.contextScreen
	case "auth":
		action = u.authForm
	case "approvals":
		action = func() {
			if u.client == nil {
				u.status("Select a project and authenticate first")
				return
			}
			u.approvalScreen()
		}
	case "refresh":
		action = func() {
			if u.client == nil {
				u.status("Select a project and authenticate first")
				return
			}
			switch u.screen {
			case "active-runs":
				u.loadActiveRunsWithProjects(true)
			case "runs":
				u.loadRuns()
			case "timeline":
				u.loadTimeline()
			case "logs":
				u.loadLog()
			case "groups":
				u.groupScreen()
			default:
				u.home()
				u.refreshPipelines()
			}
		}
	default:
		if strings.HasPrefix(strings.TrimSpace(text), ":") {
			u.status("Unknown command. Enter help to see available commands.")
			return true
		}
		return false
	}
	u.commandBar.SetText("")
	action()
	return true
}
