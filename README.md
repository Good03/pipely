# Pipely

A keyboard-driven terminal UI and CLI for Azure DevOps Services YAML pipelines.
Pipely has an original purple and charcoal interface, built with Go, Cobra,
tview and tcell. It does not incorporate K9s source code or assets.

## Install

Requires Go 1.27+ to build. Azure CLI is required for Azure login; PAT mode
works without Azure CLI or its Azure DevOps extension.

```sh
go build -o pipely ./cmd/pipely
pipely context use my-organization my-project
pipely auth login
pipely tui
```

On Windows use `go build -o pipely.exe ./cmd/pipely`.
Bare `pipely` displays CLI help; the TUI entrypoint is `pipely tui`.

## Authentication and contexts

Pipely reuses your `az login` session and obtains a token specifically for
Azure DevOps. It refreshes the cached token before expiry. You can also use
`pipely auth login --device-code`.

For PAT authentication:

```sh
pipely auth pat
```

This uses `AZURE_DEVOPS_EXT_PAT` (or `PIPELY_PAT`) when supplied, otherwise
prompts with hidden input and saves the PAT in the OS credential store.
Linux credential storage requires a running Secret Service implementation;
use an environment variable when no credential store is available.
PATs need Build read/execute access. Automatic branch and parameter discovery
also requires repository read access. Project visibility follows your ADO permissions.

```sh
pipely context use another-organization another-project
pipely context list
pipely context projects
pipely auth status
```

Organizations are supplied by name, e.g. `contoso`. The TUI can add an
organization, list its projects, and switch among saved contexts.
`--org`, `--project`, and `--auth az|pat` override the current context for a command.
PAT credentials are scoped to the organization.

Configuration is stored under the platform's user configuration directory:
`~/Library/Application Support/pipely/config.json` on macOS,
`~/.config/pipely/config.json` on Linux (or `$XDG_CONFIG_HOME`), and
`%AppData%\pipely\config.json` on Windows. Override with `--config PATH`.
Legacy configurations still load; on the next settings save, a plaintext PAT
is migrated to the credential store before being removed from the file.
Migration leaves the original file untouched if credential storage fails.

## TUI

- Arrows navigate fields and buttons; Enter opens/selects; Space toggles checkboxes.
  Esc closes dropdowns/modals first, then goes back. Tab is optional.
- F4 focuses the Pipelines / Active runs / Groups tabs. Use Left/Right and Enter;
  Selecting a tab keeps focus in the header so Left/Right can choose another tab.
  Only the area with keyboard focus gets a filled purple selection; inactive
  content retains its cursor position without a selection highlight.
  Down enters the content. Up from the first table row also focuses the tabs.
- F1 opens help; Ctrl-C quits. The footer separates status from contextual shortcuts.
- F2 copies focused log/error text or the selected table row. Ctrl-Y copies the
  complete last error. Mouse capture is off by default so terminal text selection
  works; F3 toggles it. Clipboard copying uses native OS tools with a terminal
  clipboard fallback (terminal clipboard support varies).
- Press `:` (Shift+;) from navigation to focus the command bar. It is available
  across tabs. Type `exit` and press Enter to quit. Other commands include `help`,
  `pipelines`, `active`, `groups`, `refresh`, `context`, `auth`, and `approvals`.
  Commands execute only on Enter; ordinary text filters the Pipelines table.
  Esc or Down returns to content; Up moves to the header. Colons typed in normal
  text fields are preserved.
- Pipelines has a repository pane on the left and matching pipelines in the
  middle. Left/Right (or h/l) moves between panes; Up/Down chooses repositories
  and updates the pipeline list. All repositories shows the complete project.
  Repository scope and command-bar filtering combine, and the current repository
  is retained when returning from run details. Group selections remain project-wide.
- Pipelines: `/` also focuses the command bar; `n` configures a run, `r` refreshes,
  `o` opens contexts, and `a` opens authentication.
- Space selects pipelines for a group; `s` saves the selection; `g` opens groups.
- Run configuration loads branches and parameter definitions automatically.
  Choose branches from a dropdown. Boolean parameters are checkboxes, allowed
  values are dropdowns, and string-list choices are checkboxes. Unrestricted
  strings/numbers use ordinary fields. Changing branch refreshes the schema.
  Structured parameters use pipeline defaults or allowed values; required
  structured parameters without either need a pipeline-defined default.
  TUI configuration never requires entering JSON. Options loading has a 30-second
  overall deadline, including authentication. Azure CLI token lookup is limited
  to 15 seconds; timeout errors keep Retry and branch selection available.
  If the configured YAML file is missing on the selected branch, Pipely explains
  which file is missing, keeps the branch dropdown available, and blocks launch
  until a branch with readable YAML is selected. It never substitutes YAML from
  another branch.
- Stages: Space toggles, `a` selects all, `x` clears optional stages.
  Dependencies are selected automatically and marked required.
  Deselect dependent stages before removing a required dependency.
  Enter reviews and launches; `s` saves these options to a group member.
- Runs: latest five plus active runs; Enter opens stages/jobs/tasks.
- Active runs: all queued/running pipeline runs in the current project, refreshed
  automatically. Esc from a run opened here returns to this tab.
- Failed runs show Azure's validation and stage/job/task issues, including failures
  before an agent starts. Press `e` in run details or logs to reopen diagnostics;
  F2 copies the report. Enter on a failed record without a log opens its issues.
- Run details: Enter on a record with a log ID follows its output (`f` toggles following); `c` cancels;
  `R` retries failed work (select a stage for a stage-specific retry).
- URLs in logs and diagnostics are native terminal hyperlinks. Ctrl/Cmd+click
  opens them in terminals that support OSC 8 links (mouse capture stays off by
  default). Press `o` for a keyboard link picker, then arrows and Enter to open
  a URL in your default browser. Esc closes the picker; F2 still copies plain text.
- `p` shows pending approvals in the current project and assigned approvers.
  Complete approvals in Azure DevOps. Project approvals can belong to other runs;
  Pipely does not assume that every waiting check is an approval.

Active views poll every three seconds. Azure remains the source of run history;
Pipely does not retain logs or run history offline.

## CLI

```sh
pipely repo list
pipely pipeline list
pipely pipeline list --repo my-repository
pipely pipeline metadata 42 --branch main --json
pipely run 42 --branch main --stages Prod --parameters '{"release":true}' --dry-run
pipely run 42 --branch main --stages Build,Test --watch
pipely runs active
pipely runs list 42
pipely runs status 1234
pipely runs diagnose 1234
pipely runs watch 1234
pipely logs 1234 7 --follow
pipely runs cancel 1234
pipely runs retry 1234
pipely runs retry 1234 --stage Test
pipely approvals
```

`--json` produces machine-readable output. Watch/follow commands emit successive
JSON snapshots/objects. Failed or canceled watched runs return a nonzero exit code.
Log IDs are listed by `runs status`. Azure log envelopes are decoded into lines;
structured log records are expanded for readability. The TUI highlights error,
warning, command and section markers, wraps long lines and supports copying.

Stage IDs are case-sensitive YAML identifiers, not display names. Selecting Prod
includes its transitive `dependsOn` chain, including implicit dependencies on the
previous stage. An explicit `dependsOn: []` stays independent. Stages marked
`isSkippable: false` and their dependencies are always included. Omitted `--stages`
means all stages; omitted `--branch` uses Azure's default branch.

Azure expands templates through a preview request before Pipely computes the
selection. Runtime parameters must be supplied before preview when required.
Azure conditions, permissions, environment checks and approval rules still apply.
Preview failure blocks launch; Pipely does not guess stages from previous runs.
Automatic branch/parameter discovery currently supports Azure Repos; other source
providers can use CLI branch/parameter options and Azure preview expansion.
Preview and queue are separate requests: branch/template changes between them can
change the resolved pipeline. `--dry-run` does not queue a run.

## Parallel project groups

```sh
pipely group save microservices 42 43 44
pipely group list
pipely group run microservices --dry-run
pipely group run microservices --concurrency 4
pipely group delete microservices
```

Groups belong to one organization/project. All members are validated before any
are queued, and queue requests run in parallel with a configurable concurrency
limit. Azure agent availability determines actual execution parallelism.
Submission results include each run ID or error. A failed network request may have
queued a run: inspect history before resubmitting. Pipely never automatically
retries queue requests or rolls back successfully queued members.

For per-member branches, parameters and stages, create `members.json`:

```json
[
  {"pipelineId":42,"branch":"main","stages":["Prod"],"parameters":{"release":true}},
  {"pipelineId":43,"branch":"release","stages":["Build","Test"]}
]
```

```sh
pipely group save microservices --file members.json
```

The TUI also lets you configure a pipeline and save its options into an existing
group. Group submission results link to each run's live details.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
```

Unit/integration tests use mock Azure HTTP servers, a simulated terminal and
pipeline YAML fixtures. Real ADO template expansion, deployment approvals, and
retry permissions should also be verified with a disposable pipeline before use
in a production release. No real Azure runs are created by the test suite.
