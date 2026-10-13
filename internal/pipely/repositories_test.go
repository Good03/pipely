package pipely

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestCatalogMapsPipelinesToSourceRepositories(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/build/definitions"):
			if r.URL.Query().Get("includeAllProperties") != "true" || r.URL.Query().Get("processType") != "2" {
				t.Error("full YAML definitions were not requested")
			}
			if r.URL.Query().Get("continuationToken") == "next" {
				fmt.Fprint(w, `{"value":[{"id":2,"name":"external","repository":{"id":"other-project-repo","name":"Shared","type":"TfsGit"}}]}`)
			} else {
				w.Header().Set("X-MS-ContinuationToken", "next")
				fmt.Fprint(w, `{"value":[{"id":1,"name":"orders build","repository":{"id":"orders","name":"Orders","type":"TfsGit"}}]}`)
			}
		case strings.HasSuffix(r.URL.Path, "/git/repositories"):
			fmt.Fprint(w, `{"value":[{"id":"orders","name":"Orders"},{"id":"empty","name":"Empty repo"}]}`)
		default:
			t.Error(r.URL.Path)
			http.NotFound(w, r)
		}
	})
	catalog, e := c.Catalog(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(catalog.Pipelines) != 2 || len(catalog.Repositories) != 3 {
		t.Fatalf("%+v", catalog)
	}
	values, e := FilterRepositoryPipelines(catalog.Pipelines, "Orders")
	if e != nil || len(values) != 1 || values[0].Id != 1 {
		t.Fatalf("%v %v", values, e)
	}
	values, e = FilterRepositoryPipelines(catalog.Pipelines, "other-project-repo")
	if e != nil || len(values) != 1 || values[0].Id != 2 {
		t.Fatal(values, e)
	}
}
func TestCatalogKeepsPipelinesWhenRepositoryListingForbidden(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/build/definitions") {
			fmt.Fprint(w, `{"value":[{"id":1,"name":"orders build","repository":{"id":"orders","name":"Orders"}}]}`)
		} else {
			w.WriteHeader(403)
		}
	})
	catalog, e := c.Catalog(context.Background())
	if e != nil || len(catalog.Pipelines) != 1 || len(catalog.Repositories) != 1 || catalog.Notice == "" {
		t.Fatalf("%+v %v", catalog, e)
	}
}
func TestRepositoryNameAmbiguityRequiresID(t *testing.T) {
	pipelines := []Pipeline{{Id: 1, Repository: &Repo{Id: "one", Name: "shared"}}, {Id: 2, Repository: &Repo{Id: "two", Name: "shared"}}}
	if _, e := FilterRepositoryPipelines(pipelines, "shared"); e == nil {
		t.Fatal("ambiguous name accepted")
	}
}
