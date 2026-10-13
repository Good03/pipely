package pipely

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

func (c *Client) Repositories(ctx context.Context) ([]Repo, error) {
	return listAll[Repo](ctx, c, "git/repositories", true, nil)
}
func (c *Client) RepositoryPipelines(ctx context.Context) ([]Pipeline, error) {
	return listAll[Pipeline](ctx, c, "build/definitions", true, url.Values{"includeAllProperties": {"true"}, "processType": {"2"}})
}

type PipelineCatalog struct {
	Pipelines    []Pipeline
	Repositories []Repo
	Notice       string
}

func (c *Client) Catalog(ctx context.Context) (PipelineCatalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pipelines, e := c.RepositoryPipelines(ctx)
	if e != nil {
		return PipelineCatalog{}, e
	}
	repos, e := c.Repositories(ctx)
	catalog := PipelineCatalog{Pipelines: pipelines, Repositories: catalogRepositories(repos, pipelines)}
	if e != nil {
		catalog.Notice = fmt.Sprintf("Repository listing unavailable; showing repositories referenced by pipelines. %s", e)
	}
	return catalog, nil
}
func catalogRepositories(repos []Repo, pipelines []Pipeline) []Repo {
	byID := map[string]Repo{}
	for _, r := range repos {
		if r.Id != "" {
			byID[r.Id] = r
		}
	}
	for _, p := range pipelines {
		if p.Repository == nil || p.Repository.Id == "" {
			continue
		}
		r := *p.Repository
		if old, ok := byID[r.Id]; ok {
			if old.Name == "" {
				old.Name = r.Name
			}
			if old.Type == "" {
				old.Type = r.Type
			}
			byID[r.Id] = old
		} else {
			byID[r.Id] = r
		}
	}
	result := []Repo{}
	for _, r := range byID {
		if r.Name == "" {
			r.Name = r.Id
		}
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := strings.ToLower(result[i].Name), strings.ToLower(result[j].Name)
		if a == b {
			return result[i].Id < result[j].Id
		}
		return a < b
	})
	return result
}
func filterRepositoryPipelines(pipelines []Pipeline, repoID string) []Pipeline {
	result := []Pipeline{}
	for _, p := range pipelines {
		if repoID == "" || (p.Repository != nil && p.Repository.Id == repoID) {
			result = append(result, p)
		}
	}
	return result
}
func FilterRepositoryPipelines(pipelines []Pipeline, selector string) ([]Pipeline, error) {
	if selector == "" {
		return pipelines, nil
	}
	repos := catalogRepositories(nil, pipelines)
	id := ""
	for _, r := range repos {
		if r.Id == selector {
			id = r.Id
			break
		}
		if strings.EqualFold(r.Name, selector) {
			if id != "" && id != r.Id {
				return nil, fmt.Errorf("repository name %q is ambiguous; use its ID", selector)
			}
			id = r.Id
		}
	}
	if id == "" {
		return []Pipeline{}, nil
	}
	return filterRepositoryPipelines(pipelines, id), nil
}
