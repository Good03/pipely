package pipely

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

const DevOpsResource = "499b84ac-1321-427f-aa17-267ca6975798"

type TokenSource interface {
	Authorization(context.Context) (string, error)
}
type PATSource string

func (p PATSource) Authorization(context.Context) (string, error) {
	if p == "" {
		return "", errors.New("PAT is empty")
	}
	r, _ := http.NewRequest("GET", "http://localhost", nil)
	r.SetBasicAuth("", string(p))
	return r.Header.Get("Authorization"), nil
}

type AZSource struct {
	once    sync.Once
	gate    chan struct{}
	token   string
	until   time.Time
	command func(context.Context) *exec.Cmd
}

func (a *AZSource) Authorization(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	a.once.Do(func() { a.gate = make(chan struct{}, 1) })
	select {
	case a.gate <- struct{}{}:
	case <-ctx.Done():
		return "", fmt.Errorf("Azure CLI token lookup timed out or was canceled while waiting for token refresh: %w", ctx.Err())
	}
	defer func() { <-a.gate }()
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if a.token != "" && time.Now().Before(a.until) {
		return "Bearer " + a.token, nil
	}
	cmd := exec.CommandContext(ctx, "az", "account", "get-access-token", "--resource", DevOpsResource, "--output", "json")
	if a.command != nil {
		cmd = a.command(ctx)
	}
	// Bound waiting on inherited output pipes after cancellation as well.
	cmd.WaitDelay = time.Second
	b, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("Azure CLI token lookup timed out or was canceled; retry or run pipely auth login: %w", ctx.Err())
		}
		return "", fmt.Errorf("Azure CLI token unavailable; run pipely auth login or select PAT authentication: %w", err)
	}
	var t struct {
		AccessToken string      `json:"accessToken"`
		ExpiresOn   json.Number `json:"expires_on"`
	}
	if err = json.Unmarshal(b, &t); err != nil {
		return "", fmt.Errorf("decode Azure CLI token: %w", err)
	}
	if t.AccessToken == "" {
		return "", errors.New("Azure CLI returned an empty token")
	}
	a.token = t.AccessToken
	a.until = time.Now().Add(5 * time.Minute)
	if n, e := t.ExpiresOn.Int64(); e == nil {
		a.until = time.Unix(n, 0).Add(-time.Minute)
	}
	return "Bearer " + a.token, nil
}
func AzureLogin(ctx context.Context, device bool) error {
	args := []string{"login", "--allow-no-subscriptions"}
	if device {
		args = append(args, "--use-device-code")
	}
	c := exec.CommandContext(ctx, "az", args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

type Client struct {
	BaseURL      string
	Org, Project string
	HTTP         *http.Client
	Tokens       TokenSource
}

func NewClient(c Config) (*Client, error) {
	if c.Org == "" || c.Project == "" {
		return nil, errors.New("select an organization and project with pipely context use ORG PROJECT")
	}
	var t TokenSource
	switch c.Auth {
	case "", "az":
		t = &AZSource{}
	case "pat":
		p := os.Getenv("AZURE_DEVOPS_EXT_PAT")
		if p == "" {
			p = os.Getenv("PIPELY_PAT")
		}
		if p == "" && (c.LegacyPATOrg == "" || c.LegacyPATOrg == c.Org) {
			p = c.PAT
		}
		if p == "" {
			p, _ = keyring.Get("pipely", c.Org)
		}
		if p == "" {
			return nil, errors.New("no PAT found; set AZURE_DEVOPS_EXT_PAT or run pipely auth pat")
		}
		t = PATSource(p)
	default:
		return nil, fmt.Errorf("unknown auth mode %q", c.Auth)
	}
	return &Client{BaseURL: "https://dev.azure.com", Org: c.Org, Project: c.Project, HTTP: &http.Client{Timeout: 30 * time.Second}, Tokens: t}, nil
}
func (c *Client) endpoint(path string, project bool) string {
	s := strings.TrimRight(c.BaseURL, "/") + "/" + url.PathEscape(c.Org)
	if project {
		s += "/" + url.PathEscape(c.Project)
	}
	return s + "/_apis/" + path
}

// APIError keeps status handling separate from the user-facing message.
type APIError struct {
	StatusCode                      int
	Method, Path, Status, RequestID string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Azure %s %s: %s (request %s)", e.Method, e.Path, e.Status, e.RequestID)
}

func (c *Client) request(ctx context.Context, method, path string, project bool, q url.Values, body, out any) (http.Header, error) {
	// Includes token acquisition, which happens before http.Client.Do.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if q == nil {
		q = url.Values{}
	}
	q.Set("api-version", "7.1")
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path, project)+"?"+q.Encode(), reader)
	if err != nil {
		return nil, err
	}
	auth, err := c.Tokens.Authorization(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if _, text := out.(io.Writer); text {
		req.Header.Set("Accept", "text/plain")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never echo response bodies: Azure errors can contain pipeline variables or secrets.
		return resp.Header, &APIError{StatusCode: resp.StatusCode, Method: method, Path: path, Status: resp.Status, RequestID: resp.Header.Get("X-VSS-E2EID")}
	}
	if out != nil {
		if w, ok := out.(io.Writer); ok {
			_, err = io.Copy(w, resp.Body)
		} else {
			err = json.NewDecoder(resp.Body).Decode(out)
		}
	}
	return resp.Header, err
}
func listAll[T any](ctx context.Context, c *Client, path string, project bool, q url.Values) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	all := []T{}
	seen := map[string]bool{}
	for {
		var page struct {
			Value []T `json:"value"`
		}
		h, e := c.request(ctx, "GET", path, project, q, nil, &page)
		if e != nil {
			return nil, e
		}
		all = append(all, page.Value...)
		next := h.Get("X-MS-ContinuationToken")
		if next == "" {
			return all, nil
		}
		if seen[next] {
			return nil, errors.New("Azure repeated a continuation token")
		}
		seen[next] = true
		q.Set("continuationToken", next)
	}
}
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	return listAll[Project](ctx, c, "projects", false, nil)
}
func (c *Client) Pipelines(ctx context.Context) ([]Pipeline, error) {
	return listAll[Pipeline](ctx, c, "pipelines", true, nil)
}

type Build struct {
	ID                int                `json:"id"`
	BuildNumber       string             `json:"buildNumber"`
	Status            string             `json:"status"`
	Result            string             `json:"result"`
	SourceBranch      string             `json:"sourceBranch"`
	QueueTime         string             `json:"queueTime"`
	Definition        Pipeline           `json:"definition"`
	ValidationResults []ValidationResult `json:"validationResults,omitempty"`
}

func (b Build) State() string {
	if b.Result != "" {
		return b.Result
	}
	return b.Status
}
func (c *Client) Runs(ctx context.Context, pipeline int) ([]Build, error) {
	q := url.Values{"definitions": {fmt.Sprint(pipeline)}, "$top": {"5"}, "queryOrder": {"queueTimeDescending"}}
	var page struct {
		Value []Build `json:"value"`
	}
	_, e := c.request(ctx, "GET", "build/builds", true, q, nil, &page)
	if e != nil {
		return nil, e
	}
	active, e := listAll[Build](ctx, c, "build/builds", true, url.Values{"definitions": {fmt.Sprint(pipeline)}, "statusFilter": {"inProgress,notStarted,postponed,cancelling"}})
	if e != nil {
		return nil, e
	}
	seen := map[int]bool{}
	for _, b := range page.Value {
		seen[b.ID] = true
	}
	for _, b := range active {
		if !seen[b.ID] {
			page.Value = append(page.Value, b)
		}
	}
	return page.Value, nil
}
func (c *Client) Build(ctx context.Context, id int) (Build, error) {
	var b Build
	_, e := c.request(ctx, "GET", fmt.Sprintf("build/builds/%d", id), true, nil, nil, &b)
	return b, e
}
func (c *Client) RunURL(id int) string {
	return strings.TrimRight(c.BaseURL, "/") + "/" + url.PathEscape(c.Org) + "/" + url.PathEscape(c.Project) + "/_build/results?buildId=" + fmt.Sprint(id)
}

type Record struct {
	ID               string  `json:"id"`
	ParentID         string  `json:"parentId"`
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	Identifier       string  `json:"identifier"`
	State            string  `json:"state"`
	Result           string  `json:"result"`
	Order            int     `json:"order"`
	Attempt          int     `json:"attempt"`
	Issues           []Issue `json:"issues,omitempty"`
	ErrorCount       int     `json:"errorCount,omitempty"`
	WarningCount     int     `json:"warningCount,omitempty"`
	ResultCode       string  `json:"resultCode,omitempty"`
	CurrentOperation string  `json:"currentOperation,omitempty"`
	Log              *struct {
		ID int `json:"id"`
	} `json:"log"`
}

func (r Record) Status() string {
	if r.Result != "" {
		return r.Result
	}
	return r.State
}
func (c *Client) Timeline(ctx context.Context, id int) ([]Record, error) {
	var t struct {
		Records []Record `json:"records"`
	}
	_, e := c.request(ctx, "GET", fmt.Sprintf("build/builds/%d/timeline", id), true, nil, nil, &t)
	return t.Records, e
}
func (c *Client) Log(ctx context.Context, build, log, start int) (string, error) {
	var b bytes.Buffer
	_, e := c.request(ctx, "GET", fmt.Sprintf("build/builds/%d/logs/%d", build, log), true, url.Values{"startLine": {fmt.Sprint(start)}}, nil, &b)
	return decodeLog(b.String()), e
}
func (c *Client) Cancel(ctx context.Context, id int) error {
	_, e := c.request(ctx, "PATCH", fmt.Sprintf("build/builds/%d", id), true, nil, map[string]string{"status": "cancelling"}, nil)
	return e
}
func (c *Client) Retry(ctx context.Context, id int, stage string) error {
	if stage != "" {
		_, e := c.request(ctx, "PATCH", fmt.Sprintf("build/builds/%d/stages/%s", id, url.PathEscape(stage)), true, nil, map[string]any{"state": "retry", "forceRetryAllJobs": false}, nil)
		return e
	}
	_, e := c.request(ctx, "PATCH", fmt.Sprintf("build/builds/%d", id), true, url.Values{"retry": {"true"}}, map[string]string{"status": "inProgress"}, nil)
	return e
}

type Approval struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Instructions string `json:"instructions"`
	Steps        []struct {
		AssignedApprover struct {
			DisplayName string `json:"displayName"`
		} `json:"assignedApprover"`
		Status string `json:"status"`
	} `json:"steps"`
}

func (c *Client) Approvals(ctx context.Context) ([]Approval, error) {
	return listAll[Approval](ctx, c, "pipelines/approvals", true, url.Values{"state": {"pending"}, "$expand": {"steps"}})
}

func (c *Client) ActiveRuns(ctx context.Context) ([]Build, error) {
	return listAll[Build](ctx, c, "build/builds", true, url.Values{"statusFilter": {"inProgress,notStarted,postponed,cancelling"}, "queryOrder": {"queueTimeDescending"}})
}
