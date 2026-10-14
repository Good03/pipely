package pipely

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *terminalUI) updateHints() {
	common := "[#b694ff::b]↑↓[-:-:-] Move   [#b694ff::b]Enter[-:-:-] Select   [#b694ff::b]Esc[-:-:-] Back   [#b694ff::b]F1[-:-:-] Help   [#b694ff::b]F2[-:-:-] Copy"
	actions := "Ctrl-Y Copy error   F3 Mouse capture"
	switch u.screen {
	case "pipelines":
		actions = "←→ Panes   n New run   Space Mark   s Group   g Groups"
	case "run-form", "auth", "context-form", "save-group":
		actions = "←→ Buttons   Space Toggle checkbox   Ctrl-Y Copy error"
	case "stages":
		actions = "Space Toggle   a All   x Clear   s Save to group   Ctrl-Y Copy error"
		if u.editingGroupName != "" {
			actions = "Space Toggle   a All   x Clear   Enter Save changes   Esc Back"
		}
	case "runs":
		actions = "n New run   r Refresh   Ctrl-Y Copy error"
	case "active-runs":
		actions = "← Projects   Enter Open run   r Refresh   Ctrl-Y Copy error"
	case "timeline":
		actions = "e Failure details   c Cancel   R Retry   p Approvals"
	case "logs":
		actions = "f Follow / pause   e Failures   o Links   PgUp/PgDn Scroll"
	case "groups":
		actions = "← Projects   Enter Open group   r Refresh   Ctrl-Y Copy error"
	case "contexts":
		actions = "Enter Select project   n Add organization"
	case "group-detail":
		actions = "Enter Edit pipeline   r Review launch   d Delete group"
	case "approvals":
		actions = "r Refresh   Ctrl-Y Copy error"
	}
	if front, _ := u.pages.GetFrontPage(); front != "main" {
		switch front {
		case "help", "confirm":
			actions = "←→ Buttons   ↑↓ Scroll   PgUp/PgDn Scroll   Esc Close"
		case "info":
			actions = "o Links   PgUp/PgDn Scroll   Ctrl-Y Copy error"
		case "links":
			actions = "↑↓ Choose link   Enter Open browser   Esc Close"
		default:
			actions = "←→ Choose button   Esc Close   Ctrl-Y Copy error"
		}
	}
	if u.tabs.HasFocus() {
		actions = "←→ Choose tab   Enter Open tab   ↓ Return to content"
	} else {
		if !u.choosing {
			actions += "   : Command"
		}
	}
	if u.commandBar.HasFocus() {
		actions = "Enter Execute / show matches   ↓ Content   ↑ Header   exit Quit"
	}
	shortcuts := strings.Split(actions, "   ")
	for i, shortcut := range shortcuts {
		key, description, _ := strings.Cut(shortcut, " ")
		shortcuts[i] = "[#b694ff::b]" + tview.Escape(key) + "[-:-:-] " + tview.Escape(description)
	}
	u.hints.SetText(common + "\n" + strings.Join(shortcuts, "   "))
}
func (u *terminalUI) reportError(err error) {
	text := err.Error()
	fresh := text != u.lastError
	u.lastError = text
	u.status("Error: " + text)
	if fresh {
		u.info("Error · F2 copy", text)
	}
}
func (u *terminalUI) status(s string) {
	u.footer.SetText("[#b694ff::b] STATUS [-:-:-]  " + tview.Escape(s))
}

const githubIssuesURL = "https://github.com/Good03/pipely/issues"

func (u *terminalUI) disclaimer() {
	u.footerNotice.SetText("Early version of Pipely.\nReport bugs on [::u:" + githubIssuesURL + "]GitHub[::U:-].")
}

func (u *terminalUI) footerNoticeMouse(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
	x, y := event.Position()
	if action == tview.MouseLeftClick && u.terminalScreen != nil && u.footerNotice.InRect(x, y) {
		_, _, style, _ := u.terminalScreen.GetContent(x, y)
		if style.Url("") != style && style.Url(githubIssuesURL) == style {
			u.openLink(githubIssuesURL)
		}
	}
	return action, nil
}

func (u *terminalUI) title() {
	u.header.SetText(fmt.Sprintf("[#b694ff::b] PIPELY [-:-:-]  %s / %s", tview.Escape(u.config.Org), tview.Escape(u.config.Project)))
}
