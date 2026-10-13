# Pipely

Run and monitor Azure DevOps YAML pipelines from your terminal, using an
interactive TUI or CLI.

- Browse repositories and pipelines across organizations and projects.
- Choose branches, parameters, and stages, with dependencies selected automatically.
- Follow active runs, view logs and errors, cancel runs, and retry failed work.
- Launch project-scoped groups of pipelines together.
- Authenticate with Azure CLI or a personal access token (PAT).

## Install

Download an archive from [GitHub Releases](https://github.com/Good03/pipely/releases),
extract it, and add the executable's folder to your `PATH`. Go is not required.

| OS | Platform | Archive | Executable |
| --- | --- | --- | --- |
| Windows | `windows` | `.zip` | `pipely.exe` |
| Linux | `linux` | `.tar.gz` | `pipely` |
| macOS | `darwin` | `.tar.gz` | `pipely` |

Choose `amd64` for Intel/x86-64 or `arm64` for ARM64, including Apple Silicon.
Archive names follow `pipely_VERSION_PLATFORM_ARCH`.

### Download from a terminal

Replace the example version, platform, and architecture with a published release
and your system's values.

**macOS / Linux:**

```sh
version=0.1.0
platform=darwin
arch=arm64
archive="pipely_${version}_${platform}_${arch}.tar.gz"
curl -fLO "https://github.com/Good03/pipely/releases/download/v${version}/${archive}"
tar -xzf "$archive" pipely
./pipely --help
```

**Windows (PowerShell):**

```powershell
$version = '0.1.0'
$arch = 'amd64'
$archive = "pipely_${version}_windows_${arch}.zip"
Invoke-WebRequest "https://github.com/Good03/pipely/releases/download/v${version}/${archive}" -OutFile $archive
Expand-Archive $archive -DestinationPath "pipely-$version"
& ".\pipely-$version\pipely.exe" --help
```

## Quick start

For `https://dev.azure.com/contoso/payments`:

```sh
pipely context use contoso payments
pipely auth login
pipely tui
```

Azure login requires [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli).
Pipely can also reuse an existing `az login` session. To use a PAT instead, run
`pipely auth pat` after selecting your context; Azure CLI is then optional.
PAT mode also accepts `AZURE_DEVOPS_EXT_PAT` or `PIPELY_PAT`.

PATs need Build read/execute and repository read permissions for pipeline and
metadata access. Saving a PAT on Linux requires Secret Service; use an environment
variable if no credential store is available.

## TUI

Open `pipely tui`, choose a project in the left sidebar with Enter, select a
repository and pipeline, then press `n` to configure
a run. Choose a branch and parameters, select stages, and review before launching.
Selecting a stage automatically includes its prerequisites.

| Key | Action |
| --- | --- |
| Arrows / Vim keys | Navigate lists and tables; arrows also navigate forms. |
| `Enter` / `Space` | Select / toggle. |
| `Esc` | Close a dropdown or dialog, or go back. |
| `F4` | Focus tabs; Left/Right and Enter switch tabs. |
| `:` | Open the command bar; type `exit` to quit. |
| `F2` / `Ctrl-Y` | Copy focused text / last error. |
| `F1` | Show help and more shortcuts. |

The sidebar lists accessible projects in the selected organization above the
current project's repositories. Press Up from the first repository to focus
projects; press Enter to switch. Press `r` to refresh newly created projects.

The **Active runs** and **Groups** tabs also have a project sidebar. Press Left
to focus it and Enter on a project to filter the current tab. New groups belong
to the project whose pipelines you selected; the save form shows the assignment.
In logs, press `o` to open links and `f` to toggle following.

## CLI

Replace the example pipeline ID `42` and run ID `1234` with your own:

```sh
pipely pipeline list
pipely run 42 --branch main --stages Prod --dry-run
pipely run 42 --branch main --stages Build,Test --watch
pipely runs active
pipely runs list 42
pipely runs diagnose 1234
pipely runs status 1234
pipely logs 1234 7 --follow
pipely runs cancel 1234
pipely runs retry 1234
```

Stage IDs are case-sensitive. Find log IDs with `runs status`. Use `--json` for
structured output and `pipely COMMAND --help` for options.

Run multiple pipelines from the same project:

```sh
pipely group save microservices 42 43 44
pipely group run microservices --dry-run
pipely group run microservices
```

Azure agent capacity determines actual execution parallelism. Azure conditions
and approvals still apply; complete approvals in Azure DevOps. Automatic branch
and parameter discovery supports Azure Repos.

## Build from source

Requires Go 1.27 or newer:

```sh
git clone https://github.com/Good03/pipely.git
cd pipely
go build -o pipely ./cmd/pipely
./pipely --help
```

On Windows, use `go build -o pipely.exe ./cmd/pipely`.

## License

[Apache License 2.0](LICENSE).
