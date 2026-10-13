package pipely

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMetadataFromSourceRepository(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/org/project/_apis/pipelines/7":
			fmt.Fprint(w, `{"configuration":{"path":"pipelines/build.yml","repository":{"id":"source-repo","type":"azureReposGit"}}}`)
		case "/org/_apis/git/repositories/source-repo":
			fmt.Fprint(w, `{"defaultBranch":"refs/heads/release/current"}`)
		case "/org/_apis/git/repositories/source-repo/refs":
			fmt.Fprint(w, `{"value":[{"name":"refs/heads/release/current"},{"name":"refs/heads/main"}]}`)
		case "/org/_apis/git/repositories/source-repo/items":
			if r.URL.Query().Get("path") != "/pipelines/build.yml" {
				t.Error("incorrect YAML path")
			}
			if r.URL.Query().Get("versionDescriptor.version") != "release/current" {
				t.Error("wrong default branch")
			}
			fmt.Fprint(w, `{"content":"parameters:\n- name: tests\n  displayName: Run tests\n  type: boolean\n  default: true\nstages: []\n"}`)
		default:
			t.Errorf("unexpected lookup %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	m, e := c.Metadata(context.Background(), 7, "")
	if e != nil {
		t.Fatal(e)
	}
	if m.Branch != "release/current" || len(m.Branches) != 2 || len(m.Parameters) != 1 || m.Parameters[0].Default != true {
		t.Fatalf("%+v", m)
	}
}
func TestMetadataFileFailureExplainsBranchAndPath(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "pipelines/"):
			fmt.Fprint(w, `{"configuration":{"path":"missing.yml","repository":{"id":"repo","type":"azureReposGit"}}}`)
		case strings.HasSuffix(r.URL.Path, "/refs"):
			fmt.Fprint(w, `{"value":[{"name":"refs/heads/main"}]}`)
		case strings.HasSuffix(r.URL.Path, "/items"):
			http.NotFound(w, r)
		default:
			fmt.Fprint(w, `{"defaultBranch":"refs/heads/main"}`)
		}
	})
	_, e := c.Metadata(context.Background(), 7, "")
	if e == nil || !strings.Contains(e.Error(), "missing.yml was not found on branch main") {
		t.Fatalf("%v", e)
	}
}
