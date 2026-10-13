package pipely

import (
	"context"
	"log/slog"
)

func RunPipeline(id int, c Config) {
	client, e := NewClient(c)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	p, e := client.Prepare(context.Background(), RunOptions{PipelineID: id, Branch: c.Branch})
	if e != nil {
		slog.Error(e.Error())
		return
	}
	b, e := client.Launch(context.Background(), p)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	slog.Info("Pipeline queued", "run", b.ID)
}
func RunPipelines(c Config) {
	client, e := NewClient(c)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	g := Group{Org: c.Org, Project: c.Project}
	for _, p := range c.Pipelines {
		g.Members = append(g.Members, RunOptions{PipelineID: p.Id, Branch: c.Branch})
	}
	plans, e := client.PrepareGroup(context.Background(), g)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	results, e := client.LaunchGroup(context.Background(), plans, 4)
	if e != nil {
		slog.Error(e.Error())
	}
	for _, r := range results {
		slog.Info("Group member", "pipeline", r.PipelineID, "run", r.RunID, "error", r.Error)
	}
}
