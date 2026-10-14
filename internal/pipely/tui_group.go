package pipely

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Updating membership from the pipeline browser must not reset configured
// branches, parameters, and stages for pipelines that remain in the group.
func preserveGroupOptions(existing, selected []RunOptions) []RunOptions {
	byID := make(map[int]RunOptions, len(existing))
	for _, member := range existing {
		byID[member.PipelineID] = member
	}
	result := append([]RunOptions(nil), selected...)
	for i, member := range result {
		if saved, ok := byID[member.PipelineID]; ok {
			result[i] = saved
		}
	}
	return result
}

// Commit in-memory group changes only after the configuration is saved.
func (u *terminalUI) persistGroup(g Group) error {
	next := u.config
	next.Groups = append([]Group(nil), u.config.Groups...)
	if err := PutGroup(&next, g); err != nil {
		return err
	}
	if err := SaveSettings(u.path, next); err != nil {
		return err
	}
	next.PAT, next.LegacyPATOrg = "", ""
	next.Version = 2
	u.config = next
	return nil
}

func groupMemberLabels(member RunOptions, pipelines []Pipeline, defaults map[int]string) (Pipeline, string, string) {
	pipeline := Pipeline{Id: member.PipelineID, Name: fmt.Sprintf("Pipeline #%d", member.PipelineID)}
	for _, p := range pipelines {
		if p.Id == member.PipelineID {
			pipeline = p
			break
		}
	}
	branch := strings.TrimPrefix(member.Branch, "refs/heads/")
	if branch == "" {
		branch = "Azure default branch"
		if resolved := defaults[member.PipelineID]; resolved != "" {
			branch = resolved + " (default)"
		}
	}
	stages := strings.Join(member.Stages, ", ")
	if stages == "" {
		stages = "All stages"
	}
	return pipeline, branch, stages
}

func (u *terminalUI) groupDetail(g Group) {
	u.editingGroupName = ""
	t := table()
	pipelines := append([]Pipeline(nil), u.pipelines...)
	defaults := map[int]string{}
	loadingMetadata := false
	render := func() {
		row, _ := t.GetSelection()
		t.Clear()
		headers(t, "Pipeline", "Branch", "Stages")
		for i, member := range g.Members {
			p, branch, stages := groupMemberLabels(member, pipelines, defaults)
			t.SetCell(i+1, 0, cell(p.Name).SetReference(member))
			t.SetCell(i+1, 1, cell(branch))
			t.SetCell(i+1, 2, cell(stages))
		}
		prepareTable(t)
		t.Select(max(1, row), 0)
	}
	launch := func() {
		if loadingMetadata {
			// Display-only discovery must not block the user's launch action.
			u.screenCancel()
			u.screenCtx, u.screenCancel = context.WithCancel(u.ctx)
			u.generation++
			u.busy = false
			loadingMetadata = false
		}
		cl := u.client
		if cl == nil {
			u.status("Authenticate before launching a group")
			return
		}
		u.asyncRead(func(ctx context.Context) (func(), error) {
			plans, err := cl.PrepareGroup(ctx, g)
			return func() {
				var text strings.Builder
				fmt.Fprintf(&text, "Launch %s?\nProject: %s\n\n", g.Name, g.Project)
				for _, plan := range plans {
					p, branch, _ := groupMemberLabels(plan.Options, pipelines, defaults)
					stages := strings.Join(plan.Included, ", ")
					if stages == "" {
						stages = "All jobs (implicit stage)"
					}
					fmt.Fprintf(&text, "%s\nBranch: %s\nStages: %s\n\n", p.Name, branch, stages)
				}
				u.confirm(strings.TrimSpace(text.String()), func() {
					u.async(func() (func(), error) {
						results, launchErr := cl.LaunchGroup(u.ctx, plans, 4)
						return func() { u.groupResults(g, results, launchErr) }, nil
					})
				})
			}, err
		})
	}
	deleteGroup := func() {
		u.confirm("Delete group "+g.Name+"?", func() {
			for i, x := range u.config.Groups {
				if x.Org == g.Org && x.Project == g.Project && x.Name == g.Name {
					u.config.Groups = append(u.config.Groups[:i], u.config.Groups[i+1:]...)
					break
				}
			}
			if err := u.saveSettings(); err != nil {
				u.reportError(err)
				return
			}
			u.groupScreen()
		})
	}
	t.SetSelectedFunc(func(row, _ int) {
		if value, ok := t.GetCell(row, 0).GetReference().(RunOptions); ok {
			u.editingGroupName = g.Name
			p, _, _ := groupMemberLabels(value, pipelines, defaults)
			u.runForm(p, value)
		}
	})
	actions := tview.NewForm().AddButton("Run group", launch).AddButton("Delete", deleteGroup)
	actions.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch {
		case e.Key() == tcell.KeyUp || e.Rune() == 'k':
			u.app.SetFocus(t)
			return nil
		case e.Key() == tcell.KeyLeft || e.Rune() == 'h':
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		case e.Key() == tcell.KeyRight || e.Key() == tcell.KeyDown || e.Rune() == 'l':
			return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
		case e.Rune() == 'r':
			launch()
			return nil
		case e.Rune() == 'd':
			deleteGroup()
			return nil
		}
		return e
	})
	t.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		row, _ := t.GetSelection()
		if (e.Key() == tcell.KeyDown || e.Rune() == 'j' || e.Key() == tcell.KeyTab) && row == t.GetRowCount()-1 {
			u.app.SetFocus(actions)
			return nil
		}
		switch e.Rune() {
		case 'r':
			launch()
			return nil
		case 'd':
			deleteGroup()
			return nil
		}
		return vimTable(t, e)
	})
	render()
	title := "Group · " + tview.Escape(g.Name) + " · " + tview.Escape(g.Project)
	u.show("group-detail", title, t)
	u.body.Clear().AddItem(newCenteredDialog(newDialogActions(t, actions), title), 0, 1, true)
	u.status(fmt.Sprintf("%s · %d pipelines", g.Name, len(g.Members)))
	if u.client == nil {
		return
	}
	cl := u.client
	loadingMetadata = true
	// Resolve display metadata with reads only; opening a group never previews or queues it.
	u.asyncRead(func(ctx context.Context) (func(), error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		loaded, err := cl.Pipelines(ctx)
		var mu sync.Mutex
		var wg sync.WaitGroup
		resolved := map[int]string{}
		limit := make(chan struct{}, 4)
		for _, member := range g.Members {
			if member.Branch != "" {
				continue
			}
			wg.Add(1)
			go func(member RunOptions) {
				defer wg.Done()
				select {
				case limit <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-limit }()
				metadata, _ := cl.Metadata(ctx, member.PipelineID, "")
				if metadata.DefaultBranch != "" {
					mu.Lock()
					resolved[member.PipelineID] = metadata.DefaultBranch
					mu.Unlock()
				}
			}(member)
		}
		wg.Wait()
		return func() {
			loadingMetadata = false
			if err == nil {
				pipelines = loaded
			}
			defaults = resolved
			render()
			u.status(fmt.Sprintf("%s · %d pipelines", g.Name, len(g.Members)))
		}, nil
	})
}
