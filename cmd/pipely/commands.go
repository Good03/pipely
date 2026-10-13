package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	p "github.com/Good03/pipely/internal/pipely"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var configFile, orgFlag, projectFlag, authFlag string
var asJSON bool
var rootCmd = &cobra.Command{Use: "pipely", Short: "Run and track Azure DevOps pipelines", SilenceUsage: true, SilenceErrors: true}

func settings() (p.Config, error) {
	c, e := p.ReadSettings(configFile)
	if orgFlag != "" {
		c.Org = orgFlag
	}
	if projectFlag != "" {
		c.Project = projectFlag
	}
	if authFlag != "" {
		c.Auth = authFlag
	}
	return c, e
}
func client() (*p.Client, error) {
	c, e := settings()
	if e != nil {
		return nil, e
	}
	return p.NewClient(c)
}
func output(cmd *cobra.Command, v any) error {
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	switch x := v.(type) {
	case []p.Pipeline:
		for _, a := range x {
			fmt.Fprintf(cmd.OutOrStdout(), "%-8d %s\n", a.Id, a.Name)
		}
	case []p.Repo:
		for _, r := range x {
			fmt.Fprintf(cmd.OutOrStdout(), "%-38s %s\n", r.Id, r.Name)
		}
	case []p.Project:
		for _, a := range x {
			fmt.Fprintf(cmd.OutOrStdout(), "%-38s %s\n", a.Id, a.Name)
		}
	case []p.Build:
		for _, a := range x {
			fmt.Fprintf(cmd.OutOrStdout(), "%-8d %-20s %-20s %s\n", a.ID, a.BuildNumber, a.State(), a.SourceBranch)
		}
	case []p.Record:
		for _, a := range x {
			fmt.Fprintf(cmd.OutOrStdout(), "%-9s %-24s %-16s %s\n", a.Type, a.Identifier, a.Status(), a.Name)
			for _, issue := range a.Issues {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", strings.ToUpper(issue.Type), issue.Message)
			}
			if a.Log != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  log: %d\n", a.Log.ID)
			}
		}
	case p.Plan:
		fmt.Fprintf(cmd.OutOrStdout(), "Pipeline: %d\nBranch: %s\nIncluded: %s\nSkipped: %s\n", x.Options.PipelineID, x.Options.Branch, strings.Join(x.Included, ", "), strings.Join(x.Skipped, ", "))
	default:
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
	}
	return nil
}
func numeric(s string) (int, error) {
	n, e := strconv.Atoi(s)
	if e != nil || n <= 0 {
		return 0, fmt.Errorf("expected a positive ID, got %q", s)
	}
	return n, nil
}
func split(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	a := strings.Split(s, ",")
	for i := range a {
		a[i] = strings.TrimSpace(a[i])
	}
	return a
}
func command(use, short string, args cobra.PositionalArgs, run func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: args, RunE: run}
}
func init() {
	defaultPath := p.GetConfigPath()
	rootCmd.PersistentFlags().StringVar(&configFile, "config", defaultPath, "Configuration file")
	rootCmd.PersistentFlags().StringVar(&orgFlag, "org", "", "Organization override")
	rootCmd.PersistentFlags().StringVar(&projectFlag, "project", "", "Project override")
	rootCmd.PersistentFlags().StringVar(&authFlag, "auth", "", "Authentication: az or pat")
	rootCmd.PersistentFlags().BoolVar(&asJSON, "json", false, "JSON output")
	rootCmd.AddCommand(command("tui", "Open the Pipely terminal interface", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		return p.StartTUI(cmd.Context(), c, configFile)
	}))
	auth := &cobra.Command{Use: "auth", Short: "Manage authentication"}
	rootCmd.AddCommand(auth)
	var device bool
	login := command("login", "Sign in with Azure CLI", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		if e := p.AzureLogin(cmd.Context(), device); e != nil {
			return e
		}
		c, e := settings()
		if e != nil {
			return e
		}
		c.Auth = "az"
		return p.SaveSettings(configFile, c)
	})
	login.Flags().BoolVar(&device, "device-code", false, "Use device-code authentication")
	auth.AddCommand(login)
	auth.AddCommand(command("pat", "Use a PAT from environment or securely store an entered PAT", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		if c.Org == "" {
			return fmt.Errorf("set --org or select a context first")
		}
		pat := os.Getenv("AZURE_DEVOPS_EXT_PAT")
		if pat == "" {
			pat = os.Getenv("PIPELY_PAT")
		}
		if pat == "" {
			fmt.Fprint(cmd.ErrOrStderr(), "PAT (stored in OS credential store): ")
			b, e := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(cmd.ErrOrStderr())
			if e != nil {
				return e
			}
			if e = p.StorePAT(c.Org, string(b)); e != nil {
				return e
			}
			c.PAT = ""
			c.LegacyPATOrg = ""
		}
		c.Auth = "pat"
		return p.SaveSettings(configFile, c)
	}))
	auth.AddCommand(command("status", "Check the selected authentication provider", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		cl, e := client()
		if e != nil {
			return e
		}
		_, e = cl.Tokens.Authorization(cmd.Context())
		if e != nil {
			return e
		}
		return output(cmd, map[string]any{"authenticated": true, "organization": cl.Org})
	}))
	contexts := &cobra.Command{Use: "context", Short: "Switch organization and project"}
	rootCmd.AddCommand(contexts)
	contexts.AddCommand(command("list", "List saved organization/project contexts", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		return output(cmd, c.Contexts)
	}))
	contexts.AddCommand(command("use ORG PROJECT", "Save and select an organization/project", cobra.ExactArgs(2), func(cmd *cobra.Command, a []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		c.RememberContext(a[0], a[1])
		return p.SaveSettings(configFile, c)
	}))
	contexts.AddCommand(command("projects", "List projects in the selected organization", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		if c.Project == "" {
			c.Project = "_"
		}
		cl, e := p.NewClient(c)
		if e != nil {
			return e
		}
		v, e := cl.Projects(cmd.Context())
		if e != nil {
			return e
		}
		return output(cmd, v)
	}))
	rootCmd.AddCommand(command("init ORG PROJECT", "Initialize an Azure CLI context", cobra.ExactArgs(2), func(cmd *cobra.Command, a []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		c.RememberContext(a[0], a[1])
		return p.SaveSettings(configFile, c)
	}))
	cfg := command("config", "Show configuration with credentials omitted", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		c.PAT = ""
		return output(cmd, c)
	})
	rootCmd.AddCommand(cfg)
	pipelines := &cobra.Command{Use: "pipeline", Short: "Discover pipelines and metadata"}
	rootCmd.AddCommand(pipelines)
	var repoFilter string
	pipelineList := command("list", "List project pipelines, optionally filtered by repository", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		cl, e := client()
		if e != nil {
			return e
		}
		var values []p.Pipeline
		if repoFilter == "" {
			values, e = cl.Pipelines(cmd.Context())
		} else {
			values, e = cl.RepositoryPipelines(cmd.Context())
			if e == nil {
				values, e = p.FilterRepositoryPipelines(values, repoFilter)
			}
		}
		if e != nil {
			return e
		}
		return output(cmd, values)
	})
	pipelineList.Flags().StringVar(&repoFilter, "repo", "", "Source repository ID or name")
	pipelines.AddCommand(pipelineList)
	repositories := &cobra.Command{Use: "repo", Short: "Browse project repositories"}
	rootCmd.AddCommand(repositories)
	repositories.AddCommand(command("list", "List repositories in the current project", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		cl, e := client()
		if e != nil {
			return e
		}
		values, e := cl.Repositories(cmd.Context())
		if e != nil {
			return e
		}
		return output(cmd, values)
	}))
	var metaBranch string
	metadata := command("metadata ID", "Discover branches and runtime parameter definitions", cobra.ExactArgs(1), func(cmd *cobra.Command, a []string) error {
		id, e := numeric(a[0])
		if e != nil {
			return e
		}
		cl, e := client()
		if e != nil {
			return e
		}
		m, e := cl.Metadata(cmd.Context(), id, metaBranch)
		if e != nil {
			return e
		}
		return output(cmd, m)
	})
	metadata.Flags().StringVar(&metaBranch, "branch", "", "Branch to inspect")
	pipelines.AddCommand(metadata)
	var branch, stages, parameters string
	var dry, watch bool
	run := command("run ID", "Preview and launch a pipeline with automatic dependencies", cobra.ExactArgs(1), func(cmd *cobra.Command, a []string) error {
		id, e := numeric(a[0])
		if e != nil {
			return e
		}
		cl, e := client()
		if e != nil {
			return e
		}
		params, e := p.ParseParameters(parameters)
		if e != nil {
			return e
		}
		plan, e := cl.Prepare(cmd.Context(), p.RunOptions{PipelineID: id, Branch: branch, Parameters: params, Stages: split(stages)})
		if e != nil {
			return e
		}
		if dry {
			return output(cmd, plan)
		}
		b, e := cl.Launch(cmd.Context(), plan)
		if e != nil {
			return e
		}
		if e = output(cmd, map[string]any{"runId": b.ID, "url": cl.RunURL(b.ID), "stages": plan.Included}); e != nil {
			return e
		}
		if watch {
			return watchRun(cmd, cl, b.ID)
		}
		return nil
	})
	run.Flags().StringVar(&branch, "branch", "", "Branch (Azure default when omitted)")
	run.Flags().StringVar(&stages, "stages", "", "Stage IDs, comma separated (all when omitted)")
	run.Flags().StringVar(&parameters, "parameters", "{}", "Runtime parameters as a JSON object")
	run.Flags().BoolVar(&dry, "dry-run", false, "Preview only; show dependency-expanded stages")
	run.Flags().BoolVar(&watch, "watch", false, "Watch the queued run")
	rootCmd.AddCommand(run)
	runs := &cobra.Command{Use: "runs", Short: "History, live status, cancellation and retry"}
	rootCmd.AddCommand(runs)
	runs.AddCommand(command("active", "List all active runs in the current project", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		cl, e := client()
		if e != nil {
			return e
		}
		v, e := cl.ActiveRuns(cmd.Context())
		if e != nil {
			return e
		}
		return output(cmd, v)
	}))
	runs.AddCommand(command("list PIPELINE_ID", "Show latest five runs and active runs", cobra.ExactArgs(1), func(cmd *cobra.Command, a []string) error {
		id, e := numeric(a[0])
		if e != nil {
			return e
		}
		cl, e := client()
		if e != nil {
			return e
		}
		v, e := cl.Runs(cmd.Context(), id)
		if e != nil {
			return e
		}
		return output(cmd, v)
	}))
	for _, action := range []string{"status", "watch", "cancel", "retry"} {
		var stage string
		actionCmd := command(action+" RUN_ID", action+" a run", cobra.ExactArgs(1), func(cmd *cobra.Command, a []string) error {
			id, e := numeric(a[0])
			if e != nil {
				return e
			}
			cl, e := client()
			if e != nil {
				return e
			}
			switch action {
			case "cancel":
				return cl.Cancel(cmd.Context(), id)
			case "retry":
				return cl.Retry(cmd.Context(), id, stage)
			case "watch":
				return watchRun(cmd, cl, id)
			default:
				v, e := cl.Timeline(cmd.Context(), id)
				if e != nil {
					return e
				}
				return output(cmd, v)
			}
		})
		if action == "retry" {
			actionCmd.Flags().StringVar(&stage, "stage", "", "Retry failed jobs in the given stage ID")
		}
		runs.AddCommand(actionCmd)
	}
	runs.AddCommand(command("diagnose RUN_ID", "Show validation and stage/job/task failure details", cobra.ExactArgs(1), func(cmd *cobra.Command, a []string) error {
		id, e := numeric(a[0])
		if e != nil {
			return e
		}
		cl, e := client()
		if e != nil {
			return e
		}
		d, e := cl.Diagnostics(cmd.Context(), id)
		if e != nil {
			return e
		}
		if asJSON {
			return output(cmd, d)
		}
		_, e = fmt.Fprint(cmd.OutOrStdout(), d.Report())
		return e
	}))
	var follow bool
	logs := command("logs RUN_ID LOG_ID", "Read or follow a task/job log (IDs from runs status)", cobra.ExactArgs(2), func(cmd *cobra.Command, a []string) error {
		id, e := numeric(a[0])
		if e != nil {
			return e
		}
		log, e := numeric(a[1])
		if e != nil {
			return e
		}
		cl, e := client()
		if e != nil {
			return e
		}
		start := 0
		for {
			text, e := cl.Log(cmd.Context(), id, log, start)
			if e != nil {
				return e
			}
			if asJSON {
				e = output(cmd, map[string]any{"runId": id, "logId": log, "text": text})
			} else {
				_, e = fmt.Fprint(cmd.OutOrStdout(), p.PrettyLog(text))
			}
			if e != nil {
				return e
			}
			start += strings.Count(text, "\n")
			if !follow {
				return nil
			}
			b, e := cl.Build(cmd.Context(), id)
			if e != nil {
				return e
			}
			if b.Status == "completed" {
				return nil
			}
			if e = pause(cmd.Context()); e != nil {
				return e
			}
		}
	})
	logs.Flags().BoolVar(&follow, "follow", false, "Follow log updates")
	rootCmd.AddCommand(logs)
	rootCmd.AddCommand(command("approvals", "Show pending approvals for this project", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		cl, e := client()
		if e != nil {
			return e
		}
		v, e := cl.Approvals(cmd.Context())
		if e != nil {
			return e
		}
		return output(cmd, v)
	}))
	addGroupCommands()
}
func pause(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
		return nil
	}
}
func watchRun(cmd *cobra.Command, cl *p.Client, id int) error {
	for {
		b, e := cl.Build(cmd.Context(), id)
		if e != nil {
			return e
		}
		records, e := cl.Timeline(cmd.Context(), id)
		if e != nil {
			return e
		}
		approvals, ae := cl.Approvals(cmd.Context())
		if ae != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Approval visibility unavailable:", ae)
		}
		if e = output(cmd, map[string]any{"run": b, "timeline": records, "projectPendingApprovals": approvals, "url": cl.RunURL(id)}); e != nil {
			return e
		}
		if b.Status == "completed" {
			if b.Result != "succeeded" && b.Result != "succeededWithIssues" {
				return fmt.Errorf("run %d finished: %s", id, b.Result)
			}
			return nil
		}
		if e = pause(cmd.Context()); e != nil {
			return e
		}
	}
}
func addGroupCommands() {
	groups := &cobra.Command{Use: "group", Short: "Manage parallel pipeline groups within this project"}
	rootCmd.AddCommand(groups)
	groups.AddCommand(command("list", "List groups in the current project", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		g := []p.Group{}
		for _, x := range c.Groups {
			if x.Org == c.Org && x.Project == c.Project {
				g = append(g, x)
			}
		}
		return output(cmd, g)
	}))
	var file string
	save := command("save NAME [PIPELINE_ID...]", "Create or replace a group; optionally load member options from JSON", cobra.MinimumNArgs(1), func(cmd *cobra.Command, a []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		g := p.Group{Name: a[0], Org: c.Org, Project: c.Project}
		if file != "" {
			b, e := os.ReadFile(file)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(b, &g.Members); e != nil {
				return e
			}
		} else {
			for _, s := range a[1:] {
				id, e := numeric(s)
				if e != nil {
					return e
				}
				g.Members = append(g.Members, p.RunOptions{PipelineID: id})
			}
		}
		if e = p.PutGroup(&c, g); e != nil {
			return e
		}
		return p.SaveSettings(configFile, c)
	})
	save.Flags().StringVar(&file, "file", "", "JSON file containing an array of per-pipeline run options")
	groups.AddCommand(save)
	groups.AddCommand(command("delete NAME", "Delete a saved group", cobra.ExactArgs(1), func(cmd *cobra.Command, a []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		if _, e = p.FindGroup(c, a[0]); e != nil {
			return e
		}
		for i, g := range c.Groups {
			if g.Name == a[0] && g.Org == c.Org && g.Project == c.Project {
				c.Groups = append(c.Groups[:i], c.Groups[i+1:]...)
				break
			}
		}
		return p.SaveSettings(configFile, c)
	}))
	var dry bool
	var concurrency int
	run := command("run NAME", "Validate all members, then launch in parallel", cobra.ExactArgs(1), func(cmd *cobra.Command, a []string) error {
		c, e := settings()
		if e != nil {
			return e
		}
		g, e := p.FindGroup(c, a[0])
		if e != nil {
			return e
		}
		cl, e := p.NewClient(c)
		if e != nil {
			return e
		}
		plans, e := cl.PrepareGroup(cmd.Context(), g)
		if e != nil {
			return e
		}
		if dry {
			return output(cmd, plans)
		}
		results, launchErr := cl.LaunchGroup(cmd.Context(), plans, concurrency)
		if e = output(cmd, results); e != nil {
			return e
		}
		return launchErr
	})
	run.Flags().BoolVar(&dry, "dry-run", false, "Validate and show plans without launching")
	run.Flags().IntVar(&concurrency, "concurrency", 4, "Maximum simultaneous queue requests")
	groups.AddCommand(run)
}
