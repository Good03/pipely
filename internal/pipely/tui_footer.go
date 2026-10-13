package pipely

import (
	"fmt"

	"github.com/rivo/tview"
)

func (u *terminalUI) updateHints() {
	common := "[::b]↑↓[-:-:-] Move   [::b]Enter[-:-:-] Select   [::b]Esc[-:-:-] Back   [::b]F1[-:-:-] Help   [::b]F2[-:-:-] Copy"
	actions := "Ctrl-Y Copy error   F3 Mouse capture"
	switch u.screen {
	case "pipelines":
		actions = "←→ Panes   n New run   Space Mark   s Group   g Groups"
	case "run-form", "auth", "context-form", "save-group":
		actions = "←→ Buttons   Space Toggle checkbox   Ctrl-Y Copy error"
	case "stages":
		actions = "Space Toggle   a All   x Clear   s Save to group   Ctrl-Y Copy error"
	case "runs":
		actions = "n New run   r Refresh   Ctrl-Y Copy error"
	case "active-runs":
		actions = "r Refresh   Ctrl-Y Copy error"
	case "timeline":
		actions = "e Failure details   c Cancel   R Retry   p Approvals"
	case "logs":
		actions = "f Follow / pause   e Failures   o Links   PgUp/PgDn Scroll"
	case "groups":
		actions = "Enter Open group   Ctrl-Y Copy error"
	case "approvals":
		actions = "r Refresh   Ctrl-Y Copy error"
	}
	if front, _ := u.pages.GetFrontPage(); front != "main" {
		switch front {
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
		actions += "   : Command"
	}
	if u.commandBar.HasFocus() {
		actions = "Enter Execute / show matches   ↓ Content   ↑ Header   exit Quit"
	}
	u.hints.SetText(common + "\n" + actions)
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
	u.footer.SetText("[#b694ff::b] STATUS [-:-:-] " + tview.Escape(s))
}
func (u *terminalUI) title() {
	u.header.SetText(fmt.Sprintf("[#b694ff::b] PIPELY [-:-:-]  %s / %s\n [gray]Authentication: %s · Ctrl-C quit[-]", tview.Escape(u.config.Org), tview.Escape(u.config.Project), tview.Escape(u.config.Auth)))
}
