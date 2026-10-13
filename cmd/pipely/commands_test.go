package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func executeCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	asJSON = false
	orgFlag = ""
	projectFlag = ""
	authFlag = ""
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	e := rootCmd.Execute()
	return out.String(), e
}
func TestContextAndGroupCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if _, e := executeCLI(t, "--config", path, "context", "use", "example", "services"); e != nil {
		t.Fatal(e)
	}
	if _, e := executeCLI(t, "--config", path, "group", "save", "microservices", "1", "2"); e != nil {
		t.Fatal(e)
	}
	text, e := executeCLI(t, "--config", path, "--json", "group", "list")
	if e != nil || !strings.Contains(text, "microservices") || !strings.Contains(text, "pipelineId") {
		t.Fatalf("%s %v", text, e)
	}
	if _, e = executeCLI(t, "--config", path, "context", "use", "example", "other"); e != nil {
		t.Fatal(e)
	}
	text, e = executeCLI(t, "--config", path, "group", "list")
	if e != nil || strings.Contains(text, "microservices") {
		t.Fatal("group leaked across projects")
	}
	if _, e = executeCLI(t, "--config", path, "group", "run", "microservices"); e == nil {
		t.Fatal("group from another project ran")
	}
	if _, e = executeCLI(t, "--config", path, "group", "save", "duplicate", "1", "1"); e == nil {
		t.Fatal("duplicate group members accepted")
	}
}
func TestCLIRejectsInvalidIDs(t *testing.T) {
	for _, args := range [][]string{{"run", "oops"}, {"runs", "cancel", "0"}, {"logs", "1", "-2"}} {
		if _, e := executeCLI(t, args...); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
