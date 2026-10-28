package pipely

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zalando/go-keyring"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return &Client{BaseURL: s.URL, Org: "org", Project: "project", HTTP: s.Client(), Tokens: PATSource("secret")}
}
func TestPrepareAndLaunch(t *testing.T) {
	previewCount, launchCount := 0, 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "" || pass != "secret" {
			t.Error("incorrect auth")
		}
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body["previewRun"] == true {
			previewCount++
			fmt.Fprint(w, `{"finalYaml":"stages:\n- stage: Build\n- stage: Test\n- stage: Prod\n- stage: Docs\n  dependsOn: []\n"}`)
			return
		}
		launchCount++
		if !reflect.DeepEqual(body["stagesToSkip"], []any{"Docs"}) {
			t.Errorf("skip = %v", body["stagesToSkip"])
		}
		repos := body["resources"].(map[string]any)["repositories"].(map[string]any)
		if repos["self"].(map[string]any)["refName"] != "refs/heads/release" {
			t.Error("wrong branch")
		}
		if body["templateParameters"].(map[string]any)["deploy"] != true {
			t.Error("lost parameter")
		}
		fmt.Fprint(w, `{"id":42}`)
	})
	p, e := c.Prepare(context.Background(), RunOptions{PipelineID: 7, Branch: "release", Stages: []string{"Prod"}, Parameters: map[string]any{"deploy": true}})
	if e != nil {
		t.Fatal(e)
	}
	if launchCount != 0 || previewCount != 1 {
		t.Fatal("preview created a run")
	}
	if !reflect.DeepEqual(p.Included, []string{"Build", "Test", "Prod"}) {
		t.Fatal(p.Included)
	}
	b, e := c.Launch(context.Background(), p)
	if e != nil || b.ID != 42 {
		t.Fatalf("%v %v", b, e)
	}
}
func TestPaginationAndErrors(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("continuationToken") == "next" {
			fmt.Fprint(w, `{"value":[{"id":2,"name":"Second"}]}`)
		} else {
			w.Header().Set("X-MS-ContinuationToken", "next")
			fmt.Fprint(w, `{"value":[{"id":1,"name":"First"}]}`)
		}
	})
	ps, e := c.Pipelines(context.Background())
	if e != nil || len(ps) != 2 {
		t.Fatalf("%v %v", ps, e)
	}
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); fmt.Fprint(w, "secret response") })
	_, e = c.Pipelines(context.Background())
	if e == nil || strings.Contains(e.Error(), "secret response") {
		t.Fatalf("unsafe error %v", e)
	}
}
func TestGroupPreflightAndPartialFailure(t *testing.T) {
	var mu sync.Mutex
	launched := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["previewRun"] == true {
			if strings.Contains(r.URL.Path, "/99/") {
				w.WriteHeader(400)
				return
			}
			fmt.Fprint(w, `{"finalYaml":"stages:\n- stage: Build\n"}`)
			return
		}
		mu.Lock()
		launched++
		mu.Unlock()
		if strings.Contains(r.URL.Path, "/2/") {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, `{"id":100}`)
	})
	g := Group{Org: "org", Project: "project", Members: []RunOptions{{PipelineID: 1}, {PipelineID: 99}}}
	if _, e := c.PrepareGroup(context.Background(), g); e == nil {
		t.Fatal("bad preview accepted")
	}
	if launched != 0 {
		t.Fatal("preflight queued runs")
	}
	g.Members[1].PipelineID = 2
	plans, e := c.PrepareGroup(context.Background(), g)
	if e != nil {
		t.Fatal(e)
	}
	results, e := c.LaunchGroup(context.Background(), plans, 2)
	if e == nil {
		t.Fatal("partial failure not returned")
	}
	if results[0].RunID != 100 || results[1].Error == "" {
		t.Fatal(results)
	}
	if launched != 2 {
		t.Fatalf("submitted %d times", launched)
	}
	g.Project = "another"
	if _, e = c.PrepareGroup(context.Background(), g); e == nil {
		t.Fatal("cross-project group accepted")
	}
}
func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	c := Config{Auth: "az"}
	c.RememberContext("one", "project")
	c.RememberContext("two", "project")
	c.RememberContext("one", "project")
	if len(c.Contexts) != 2 {
		t.Fatal(c.Contexts)
	}
	if e := SaveSettings(path, c); e != nil {
		t.Fatal(e)
	}
	got, e := ReadSettings(path)
	if e != nil || got.Org != "one" || got.Version != 2 {
		t.Fatalf("%v %v", got, e)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("configuration is readable by other users")
	}
	if e := PutGroup(&c, Group{Name: "services", Org: "one", Project: "project", Members: []RunOptions{{PipelineID: 1}, {PipelineID: 1}}}); e == nil {
		t.Fatal("duplicate member accepted")
	}
}
func TestLatestFivePlusActive(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("statusFilter") != "" {
			fmt.Fprint(w, `{"value":[{"id":1,"status":"inProgress"},{"id":8,"status":"inProgress"}]}`)
		} else {
			if r.URL.Query().Get("$top") != "5" {
				t.Error("history limit missing")
			}
			fmt.Fprint(w, `{"value":[{"id":5},{"id":4},{"id":3},{"id":2},{"id":1}]}`)
		}
	})
	runs, e := c.Runs(context.Background(), 4)
	if e != nil || len(runs) != 6 {
		t.Fatalf("%v %v", runs, e)
	}
}
func TestRetryAndCancelPayloads(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Error(r.Method)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(r.URL.Path, "/stages/Prod") {
			if body["state"] != "retry" || body["forceRetryAllJobs"] != false {
				t.Error(body)
			}
		} else if r.URL.Query().Get("retry") == "true" {
			if body["status"] != "inProgress" {
				t.Error(body)
			}
		} else if body["status"] != "cancelling" {
			t.Error(body)
		}
	})
	if e := c.Retry(context.Background(), 1, "Prod"); e != nil {
		t.Fatal(e)
	}
	if e := c.Retry(context.Background(), 1, ""); e != nil {
		t.Fatal(e)
	}
	if e := c.Cancel(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
}

func TestLegacyCredentialMigrationIsOrganizationScoped(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "")
	t.Setenv("PIPELY_PAT", "")
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"org":"original","project":"service","pat":"legacy-secret"}`)
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	c, e := ReadSettings(path)
	if e != nil {
		t.Fatal(e)
	}
	c.RememberContext("different", "service")
	if _, e = NewClient(c); e == nil {
		t.Fatal("legacy credential used in another organization")
	}
	if e = SaveSettings(path, c); e != nil {
		t.Fatal(e)
	}
	if value, e := keyring.Get("pipely", "original"); e != nil || value != "legacy-secret" {
		t.Fatal("credential was migrated to the wrong organization")
	}
	if _, e = keyring.Get("pipely", "different"); e == nil {
		t.Fatal("credential leaked into selected organization")
	}
	saved, _ := os.ReadFile(path)
	if strings.Contains(string(saved), "legacy-secret") {
		t.Fatal("plaintext secret persisted")
	}
}
func TestFailedMigrationPreservesOriginal(t *testing.T) {
	keyring.MockInitWithError(errors.New("credential store unavailable"))
	t.Cleanup(keyring.MockInit)
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"org":"original","project":"service","pat":"legacy-secret"}`)
	os.WriteFile(path, original, 0600)
	c, e := ReadSettings(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = SaveSettings(path, c); e == nil {
		t.Fatal("migration unexpectedly succeeded")
	}
	saved, _ := os.ReadFile(path)
	if string(saved) != string(original) {
		t.Fatal("migration damaged original settings")
	}
}
func TestPATEnvironmentAndExplicitAuth(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "env-secret")
	t.Setenv("PIPELY_PAT", "")
	c := Config{Org: "example", Project: "service", Auth: "az"}
	cl, e := NewClient(c)
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := cl.Tokens.(*AZSource); !ok {
		t.Fatal("PAT silently overrode Azure authentication")
	}
	c.Auth = "pat"
	cl, e = NewClient(c)
	if e != nil {
		t.Fatal(e)
	}
	if cl.Tokens.(PATSource) != "env-secret" {
		t.Fatal("environment PAT not used")
	}
}

func TestActiveRunsUsesCurrentProject(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/project/_apis/build/builds" || r.URL.Query().Get("definitions") != "" || !strings.Contains(r.URL.Query().Get("statusFilter"), "inProgress") {
			t.Error("active runs were not queried for the whole project")
		}
		fmt.Fprint(w, `{"value":[{"id":4,"status":"inProgress","definition":{"id":7,"name":"orders"}}]}`)
	})
	runs, e := c.ActiveRuns(context.Background())
	if e != nil || len(runs) != 1 || runs[0].Definition.Name != "orders" {
		t.Fatalf("%v %v", runs, e)
	}
}

func TestAzureTokenProcessHelper(t *testing.T) {
	switch os.Getenv("PIPELY_TOKEN_TEST_PROCESS") {
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "success":
		fmt.Fprint(os.Stdout, `{"accessToken":"test-token"}`)
		os.Exit(0)
	}
}
func tokenTestCommand(ctx context.Context, mode string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAzureTokenProcessHelper$")
	cmd.Env = append(os.Environ(), "PIPELY_TOKEN_TEST_PROCESS="+mode)
	return cmd
}
func TestAzureTokenCommandTimeoutReleasesGate(t *testing.T) {
	source := &AZSource{command: func(ctx context.Context) *exec.Cmd { return tokenTestCommand(ctx, "hang") }}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, e := source.Authorization(ctx)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("token request did not time out: %v", e)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("canceled token subprocess did not exit promptly")
	}
	source.command = func(ctx context.Context) *exec.Cmd { return tokenTestCommand(ctx, "success") }
	retryCtx, retryCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer retryCancel()
	token, e := source.Authorization(retryCtx)
	if e != nil || token != "Bearer test-token" {
		t.Fatalf("retry did not recover: %s %v", token, e)
	}
}
func TestAzureTokenWaitHonorsCancellation(t *testing.T) {
	source := &AZSource{}
	source.once.Do(func() { source.gate = make(chan struct{}, 1) })
	source.gate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, e := source.Authorization(ctx)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("waiting for token refresh ignored deadline: %v", e)
	}
	<-source.gate
}
func TestMetadataHonorsOverallDeadline(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, e := c.Metadata(ctx, 7, "feat/CLI")
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("metadata ignored its context deadline: %v", e)
	}
}

func TestGroupSharedBranchOverridesMembersForPreviewAndLaunch(t *testing.T) {
	requests := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Preview   bool `json:"previewRun"`
			Resources struct {
				Repositories map[string]struct {
					Ref string `json:"refName"`
				} `json:"repositories"`
			} `json:"resources"`
			Parameters map[string]any `json:"templateParameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Resources.Repositories["self"].Ref != "refs/heads/release" || body.Parameters["tests"] != true {
			t.Errorf("wrong group run options: %+v", body)
		}
		requests++
		if body.Preview {
			fmt.Fprint(w, `{"finalYaml":"stages:\n- stage: Build\n- stage: Test\n"}`)
		} else {
			fmt.Fprint(w, `{"id":100}`)
		}
	})
	g := Group{Org: "org", Project: "project", Branch: "release", Members: []RunOptions{
		{PipelineID: 1, Branch: "old", Stages: []string{"Build"}, Parameters: map[string]any{"tests": true}},
		{PipelineID: 2, Parameters: map[string]any{"tests": true}},
	}}
	plans, err := c.PrepareGroup(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if g.Members[0].Branch != "old" || len(plans[0].Included) != 1 || len(plans[1].Included) != 2 {
		t.Fatal("shared branch changed saved member settings")
	}
	for _, plan := range plans {
		if plan.Options.Branch != "release" {
			t.Fatal("plan did not use shared branch")
		}
		if _, err := c.Launch(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 4 {
		t.Fatalf("got %d preview/launch requests", requests)
	}
}
