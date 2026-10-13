package pipely

import (
	"context"
	"fmt"
	"strings"
)

type Issue struct {
	Type     string `json:"type"`
	Category string `json:"category,omitempty"`
	Message  string `json:"message"`
}
type ValidationResult struct {
	Result  string `json:"result"`
	Message string `json:"message"`
}
type RunDiagnostics struct {
	Run           Build    `json:"run"`
	Records       []Record `json:"records"`
	TimelineError string   `json:"timelineError,omitempty"`
	URL           string   `json:"url"`
}

func (c *Client) Diagnostics(ctx context.Context, id int) (RunDiagnostics, error) {
	b, e := c.Build(ctx, id)
	if e != nil {
		return RunDiagnostics{}, e
	}
	records, e := c.Timeline(ctx, id)
	d := RunDiagnostics{Run: b, Records: records, URL: c.RunURL(id)}
	if e != nil {
		d.TimelineError = e.Error()
	}
	return d, nil
}
func (d RunDiagnostics) Report() string {
	var out strings.Builder
	fmt.Fprintf(&out, "Run %d · %s\nPipeline: %s\nBranch: %s\n\n", d.Run.ID, d.Run.State(), d.Run.Definition.Name, d.Run.SourceBranch)
	messages := 0
	for _, v := range d.Run.ValidationResults {
		if v.Message != "" {
			fmt.Fprintf(&out, "%s · Pipeline validation\n%s\n\n", strings.ToUpper(v.Result), v.Message)
			messages++
		}
	}
	for _, r := range orderedRecords(d.Records) {
		if len(r.Issues) == 0 && r.Result != "failed" && r.Result != "abandoned" && r.ErrorCount == 0 {
			continue
		}
		fmt.Fprintf(&out, "%s · %s · %s (attempt %d)\n", r.Type, strings.TrimSpace(r.Name), r.Status(), r.Attempt)
		for _, issue := range r.Issues {
			fmt.Fprintf(&out, "%s: %s\n", strings.ToUpper(issue.Type), issue.Message)
			messages++
		}
		if r.ResultCode != "" {
			fmt.Fprintf(&out, "Result code: %s\n", r.ResultCode)
			messages++
		}
		if r.CurrentOperation != "" {
			fmt.Fprintf(&out, "Operation: %s\n", r.CurrentOperation)
		}
		if len(r.Issues) == 0 && r.ErrorCount > 0 {
			fmt.Fprintf(&out, "Azure reports %d errors without issue text on this record.\n", r.ErrorCount)
		}
		if r.Log != nil {
			fmt.Fprintf(&out, "Log ID: %d\n", r.Log.ID)
		} else {
			out.WriteString("No task log is available for this record.\n")
		}
		out.WriteByte('\n')
	}
	if d.TimelineError != "" {
		fmt.Fprintf(&out, "Timeline unavailable: %s\n\n", d.TimelineError)
	}
	if messages == 0 {
		out.WriteString("Azure did not return a detailed failure message in run validation or timeline issues. Inspect the failed task log and Azure run summary.\n\n")
	}
	fmt.Fprintln(&out, d.URL)
	return out.String()
}
