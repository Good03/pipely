package pipely

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDiagnosticsExposeFailureWithoutTaskLog(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/timeline") {
			fmt.Fprint(w, `{"records":[{"id":"job","name":"Job","type":"Job","result":"failed","errorCount":1,"attempt":1,"issues":[{"type":"error","message":"No hosted parallelism has been purchased or granted."}]}]}`)
		} else {
			fmt.Fprint(w, `{"id":8,"status":"completed","result":"failed","sourceBranch":"refs/heads/feat/CLI","definition":{"id":1,"name":"Example pipeline 1"}}`)
		}
	})
	d, e := c.Diagnostics(context.Background(), 8)
	if e != nil {
		t.Fatal(e)
	}
	report := d.Report()
	if !strings.Contains(report, "No hosted parallelism") || !strings.Contains(report, "No task log is available") {
		t.Fatalf("%s", report)
	}
	if strings.Contains(report, "did not return a detailed failure") {
		t.Fatal("failure incorrectly reported as unknown")
	}
}
func TestValidationSurvivesMissingTimeline(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/timeline") {
			http.NotFound(w, r)
		} else {
			fmt.Fprint(w, `{"id":9,"result":"failed","validationResults":[{"result":"error","message":"Service connection is not authorized."}]}`)
		}
	})
	d, e := c.Diagnostics(context.Background(), 9)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(d.Report(), "Service connection is not authorized") || d.TimelineError == "" {
		t.Fatal("validation error was hidden by missing timeline")
	}
}
func TestUnknownFailureDoesNotInventCause(t *testing.T) {
	d := RunDiagnostics{Run: Build{ID: 1, Result: "failed"}, Records: []Record{{ID: "task", Name: "Build", Type: "Task", Result: "failed"}}}
	if !strings.Contains(d.Report(), "Azure did not return a detailed failure message") {
		t.Fatal("missing-message limitation not shown")
	}
}
