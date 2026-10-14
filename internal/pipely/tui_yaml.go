package pipely

import (
	"context"

	"github.com/rivo/tview"
)

func (u *terminalUI) pipelineYAML(p Pipeline) {
	if u.client == nil {
		u.status("Select a project and authenticate to load YAML")
		return
	}
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(false)
	u.show("yaml", "YAML · "+tview.Escape(p.Name), view)
	u.status("Loading pipeline YAML…")
	cl := u.client
	u.asyncRead(func(ctx context.Context) (func(), error) {
		metadata, err := cl.Metadata(ctx, p.Id, "")
		return func() {
			view.SetText(tview.Escape(metadata.YAML))
			u.status("Branch: " + metadata.Branch)
		}, err
	})
}
