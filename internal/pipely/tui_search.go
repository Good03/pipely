package pipely

import "strings"

func matchesSearch(name, query string) bool {
	return strings.Contains(strings.ToLower(name), strings.ToLower(query))
}

func (u *terminalUI) beginSearch() bool {
	focus := u.app.GetFocus()
	var target, query string
	switch {
	case focus == u.projectTable:
		target, query = "Projects", u.projectFilter
	case focus == u.repoTable && u.screen == "pipelines":
		target, query = "Repositories", u.repositoryFilter
	case focus == u.pipelineTable && u.screen == "pipelines":
		target, query = "Pipelines", u.pipelineFilter
	default:
		return false
	}
	u.searchTarget, u.searchFocus = target, focus
	u.commandBar.SetLabel(" / " + target + " ").SetPlaceholder("Search by name")
	u.commandBar.SetText(query)
	u.app.SetFocus(u.commandBar)
	return true
}

func (u *terminalUI) applySearch(text string) {
	switch u.searchTarget {
	case "Projects":
		u.projectFilter = text
		u.renderProjects()
	case "Repositories":
		u.repositoryFilter = text
		u.renderRepositories()
	case "Pipelines":
		u.pipelineFilter = text
		u.renderPipelines()
	}
}

func (u *terminalUI) leaveSearch(clear bool) {
	if clear {
		u.commandBar.SetText("")
	}
	if u.searchFocus != nil {
		u.app.SetFocus(u.searchFocus)
	}
}

func (u *terminalUI) resetSearchMode() {
	wasSearch := u.searchTarget != ""
	u.searchTarget = ""
	u.searchFocus = nil
	if u.commandBar != nil {
		if wasSearch {
			u.updatingCommandBar = true
			u.commandBar.SetText("")
			u.updatingCommandBar = false
		}
		u.commandBar.SetLabel(" : Command ").SetPlaceholder("Type to filter pipelines; Enter executes exit, help, active, groups…")
	}
}
