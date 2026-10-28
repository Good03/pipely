package pipely

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type RunOptions struct {
	PipelineID int            `json:"pipelineId"`
	Branch     string         `json:"branch,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Stages     []string       `json:"stages,omitempty"`
}
type Stage struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Dependencies []string `json:"dependencies"`
	Condition    string   `json:"condition,omitempty"`
	Skippable    bool     `json:"skippable"`
}
type Parameter struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"displayName" json:"displayName,omitempty"`
	Type        string `yaml:"type" json:"type"`
	Default     any    `yaml:"default" json:"default"`
	Values      []any  `yaml:"values" json:"values,omitempty"`
}
type Metadata struct {
	Branches      []string    `json:"branches"`
	Branch        string      `json:"branch"`
	DefaultBranch string      `json:"defaultBranch"`
	Parameters    []Parameter `json:"parameters"`
	YAML          string      `json:"-"`
	Problem       string      `json:"-"`
}
type Plan struct {
	Options  RunOptions     `json:"options"`
	Stages   []Stage        `json:"stages"`
	Included []string       `json:"included"`
	Skipped  []string       `json:"skipped"`
	Request  map[string]any `json:"-"`
}

var stageID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ParseStages(text string) ([]Stage, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(text), &root); err != nil {
		return nil, err
	}
	raw, exists := root["stages"]
	if !exists {
		if root["jobs"] != nil || root["steps"] != nil {
			return []Stage{}, nil
		}
		return nil, fmt.Errorf("expanded YAML has no stages, jobs or steps")
	}
	seq, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expanded stages must be a sequence")
	}
	stages := []Stage{}
	seen := map[string]bool{}
	for i, v := range seq {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("stage %d is unresolved", i+1)
		}
		id, _ := m["stage"].(string)
		if !stageID.MatchString(id) || seen[id] {
			return nil, fmt.Errorf("invalid, duplicate or unresolved stage ID %q", id)
		}
		seen[id] = true
		s := Stage{ID: id, Name: id, Skippable: true}
		if n, ok := m["displayName"].(string); ok {
			s.Name = n
		}
		s.Condition, _ = m["condition"].(string)
		if b, exists := m["isSkippable"]; exists {
			var ok bool
			s.Skippable, ok = b.(bool)
			if !ok {
				return nil, fmt.Errorf("unresolved isSkippable in %s", id)
			}
		}
		dep, explicit := m["dependsOn"]
		if !explicit {
			if i > 0 {
				s.Dependencies = []string{stages[i-1].ID}
			}
		} else {
			switch x := dep.(type) {
			case nil:
			case string:
				if x != "" {
					s.Dependencies = []string{x}
				}
			case []any:
				for _, d := range x {
					n, ok := d.(string)
					if !ok {
						return nil, fmt.Errorf("unresolved dependency in %s", id)
					}
					s.Dependencies = append(s.Dependencies, n)
				}
			default:
				return nil, fmt.Errorf("unresolved dependencies in %s", id)
			}
		}
		stages = append(stages, s)
	}
	// Validate every node, including unselected branches, before queuing anything.
	_, _, err := ResolveStages(stages, allStageIDs(stages))
	return stages, err
}
func allStageIDs(stages []Stage) []string {
	ids := []string{}
	for _, s := range stages {
		ids = append(ids, s.ID)
	}
	return ids
}
func ResolveStages(stages []Stage, selected []string) (included, skipped []string, err error) {
	nodes := map[string]Stage{}
	for _, s := range stages {
		nodes[s.ID] = s
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		s, ok := nodes[id]
		if !ok {
			return fmt.Errorf("unknown stage %q", id)
		}
		if state[id] == 1 {
			return fmt.Errorf("stage dependency cycle at %s", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, d := range s.Dependencies {
			if e := visit(d); e != nil {
				return e
			}
		}
		state[id] = 2
		return nil
	}
	for _, s := range stages {
		if !s.Skippable {
			if e := visit(s.ID); e != nil {
				return nil, nil, e
			}
		}
	}
	for _, id := range selected {
		if e := visit(id); e != nil {
			return nil, nil, e
		}
	}
	included = []string{}
	skipped = []string{}
	for _, s := range stages {
		if state[s.ID] == 2 {
			included = append(included, s.ID)
		} else {
			skipped = append(skipped, s.ID)
		}
	}
	return
}
func (c *Client) Metadata(ctx context.Context, id int, branch string) (Metadata, error) {
	// One budget for configuration, token acquisition, branch pagination and YAML.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var pipeline struct {
		Configuration struct {
			Path       string `json:"path"`
			Repository struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"repository"`
		} `json:"configuration"`
	}
	if _, e := c.request(ctx, "GET", fmt.Sprintf("pipelines/%d", id), true, nil, nil, &pipeline); e != nil {
		return Metadata{}, fmt.Errorf("load pipeline configuration: %w", e)
	}
	cfg := pipeline.Configuration
	if cfg.Repository.Type != "azureReposGit" && cfg.Repository.Type != "TfsGit" {
		return Metadata{}, fmt.Errorf("branch and parameter discovery requires an Azure Repos YAML pipeline (repository type %q)", cfg.Repository.Type)
	}
	if cfg.Repository.ID == "" || cfg.Path == "" {
		return Metadata{}, fmt.Errorf("pipeline %d has no YAML path or repository ID", id)
	}
	repoPath := "git/repositories/" + url.PathEscape(cfg.Repository.ID)
	var repo struct {
		DefaultBranch string `json:"defaultBranch"`
	}
	if _, e := c.request(ctx, "GET", repoPath, false, nil, nil, &repo); e != nil {
		return Metadata{}, fmt.Errorf("load source repository (check repository read access): %w", e)
	}
	refs, e := listAll[struct {
		Name string `json:"name"`
	}](ctx, c, repoPath+"/refs", false, url.Values{"filter": {"heads/"}})
	if e != nil {
		return Metadata{}, fmt.Errorf("list repository branches: %w", e)
	}
	m := Metadata{DefaultBranch: strings.TrimPrefix(repo.DefaultBranch, "refs/heads/")}
	for _, r := range refs {
		m.Branches = append(m.Branches, strings.TrimPrefix(r.Name, "refs/heads/"))
	}
	sort.Strings(m.Branches)
	if branch == "" {
		branch = m.DefaultBranch
	}
	branch = strings.TrimPrefix(branch, "refs/heads/")
	m.Branch = branch
	if branch == "" {
		return m, fmt.Errorf("repository has no default branch")
	}
	path := "/" + strings.TrimLeft(cfg.Path, "/")
	var item struct {
		Content string `json:"content"`
	}
	_, e = c.request(ctx, "GET", repoPath+"/items", false, url.Values{"path": {path}, "includeContent": {"true"}, "versionDescriptor.version": {branch}, "versionDescriptor.versionType": {"branch"}}, nil, &item)
	if e != nil {
		var apiErr *APIError
		if errors.As(e, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return m, fmt.Errorf("Pipeline YAML %s was not found on branch %s. Choose a branch containing this file.\n\nRepository: %s\nIf the file exists, check repository read permissions.\nDetails: %w", path, branch, cfg.Repository.ID, e)
		}
		return m, fmt.Errorf("read pipeline YAML %s on branch %s (check file existence and repository access): %w", path, branch, e)
	}
	if item.Content == "" {
		return m, fmt.Errorf("pipeline YAML %s on %s is empty", path, branch)
	}
	m.YAML = item.Content
	var raw struct {
		Parameters []Parameter `yaml:"parameters"`
	}
	if e = yaml.Unmarshal([]byte(item.Content), &raw); e != nil {
		return m, fmt.Errorf("read parameter schema: %w", e)
	}
	m.Parameters = raw.Parameters
	return m, nil
}
func (c *Client) Prepare(ctx context.Context, opts RunOptions) (Plan, error) {
	if opts.PipelineID <= 0 {
		return Plan{}, fmt.Errorf("pipeline ID must be positive")
	}
	body := map[string]any{"previewRun": true, "templateParameters": opts.Parameters}
	if opts.Branch != "" {
		ref := opts.Branch
		if !strings.HasPrefix(ref, "refs/") {
			ref = "refs/heads/" + ref
		}
		body["resources"] = map[string]any{"repositories": map[string]any{"self": map[string]string{"refName": ref}}}
	}
	var preview struct {
		FinalYAML string `json:"finalYaml"`
	}
	_, err := c.request(ctx, "POST", fmt.Sprintf("pipelines/%d/runs", opts.PipelineID), true, nil, body, &preview)
	if err != nil {
		return Plan{}, fmt.Errorf("preview pipeline %d: %w", opts.PipelineID, err)
	}
	if preview.FinalYAML == "" {
		return Plan{}, fmt.Errorf("Azure preview returned no expanded YAML")
	}
	stages, err := ParseStages(preview.FinalYAML)
	if err != nil {
		return Plan{}, err
	}
	selected := opts.Stages
	if len(selected) == 0 {
		selected = allStageIDs(stages)
	}
	inc, skip, err := ResolveStages(stages, selected)
	if err != nil {
		return Plan{}, err
	}
	body["previewRun"] = false
	body["stagesToSkip"] = skip
	return Plan{Options: opts, Stages: stages, Included: inc, Skipped: skip, Request: body}, nil
}
func (c *Client) Launch(ctx context.Context, p Plan) (Build, error) {
	var run struct {
		ID int `json:"id"`
	}
	_, err := c.request(ctx, "POST", fmt.Sprintf("pipelines/%d/runs", p.Options.PipelineID), true, nil, p.Request, &run)
	if err != nil {
		return Build{}, fmt.Errorf("queue result may be unknown; check Azure run history before retrying: %w", err)
	}
	return Build{ID: run.ID, Definition: Pipeline{Id: p.Options.PipelineID}, Status: "notStarted"}, nil
}

type GroupResult struct {
	PipelineID int    `json:"pipelineId"`
	RunID      int    `json:"runId,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (c *Client) PrepareGroup(ctx context.Context, g Group) ([]Plan, error) {
	if g.Org != c.Org || g.Project != c.Project {
		return nil, fmt.Errorf("group belongs to another organization/project")
	}
	if len(g.Members) == 0 {
		return nil, fmt.Errorf("group is empty")
	}
	plans := []Plan{}
	seen := map[int]bool{}
	for _, m := range g.Members {
		if seen[m.PipelineID] {
			return nil, fmt.Errorf("duplicate pipeline %d", m.PipelineID)
		}
		seen[m.PipelineID] = true
		if g.Branch != "" {
			m.Branch = g.Branch
		}
		p, e := c.Prepare(ctx, m)
		if e != nil {
			return nil, e
		}
		plans = append(plans, p)
	}
	return plans, nil
}
func (c *Client) LaunchGroup(ctx context.Context, plans []Plan, concurrency int) ([]GroupResult, error) {
	if concurrency < 1 {
		return nil, fmt.Errorf("concurrency must be positive")
	}
	results := make([]GroupResult, len(plans))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, p := range plans {
		wg.Add(1)
		go func(i int, p Plan) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			b, e := c.Launch(ctx, p)
			r := GroupResult{PipelineID: p.Options.PipelineID, RunID: b.ID}
			if e != nil {
				r.Error = e.Error()
			}
			results[i] = r
		}(i, p)
	}
	wg.Wait()
	for _, r := range results {
		if r.Error != "" {
			return results, fmt.Errorf("some group members could not be queued; inspect results before retrying")
		}
	}
	return results, nil
}

// ParseParameters rejects null and arrays so both interfaces use identical inputs.
func ParseParameters(text string) (map[string]any, error) {
	var values map[string]any
	if e := json.Unmarshal([]byte(text), &values); e != nil {
		return nil, fmt.Errorf("parameters must be a JSON object: %w", e)
	}
	if values == nil {
		return nil, fmt.Errorf("parameters must be a JSON object")
	}
	return values, nil
}
