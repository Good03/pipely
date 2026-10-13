package pipely

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAzureLogEnvelope(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/plain" {
			t.Error("log requested as JSON")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"count":2,"value":["Starting build","Build succeeded"]}`)
	})
	text, e := c.Log(context.Background(), 1, 2, 0)
	if e != nil {
		t.Fatal(e)
	}
	if text != "Starting build\nBuild succeeded\n" {
		t.Fatalf("%q", text)
	}
}
func TestPrettyStructuredLogs(t *testing.T) {
	text := PrettyLog("2026-10-13T00:00:00Z {\"level\":\"info\",\"message\":\"ready\"}\nordinary output\n")
	if !strings.Contains(text, "\n  \"level\": \"info\"") || !strings.Contains(text, "ordinary output") {
		t.Fatalf("%s", text)
	}
	if got := PrettyLog("plain text\n"); got != "plain text\n" {
		t.Fatal(got)
	}
}
