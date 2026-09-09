# Pipely

A command-line tool for managing and running Azure DevOps pipelines efficiently.

Pipely simplifies interaction with Azure DevOps by providing an intuitive CLI for configuration management, and pipeline execution.

## Prerequisites

Before you start, you need:

1. **Go** version 1.27.0 or later
2. **Azure DevOps Account** with at least one organization
3. **Personal Access Token (PAT)** from Azure DevOps

### Getting a Personal Access Token

1. Go to your Azure DevOps organization
2. Click **User Settings** (profile icon)
3. Select **Personal access tokens**
4. Click **New Token**
5. Configure the token:
   - Scopes: Select at least "Build (read & execute)"
6. Click **Create** and **copy the token**

## Installation

### Option 1: Build from Source

```bash
git clone https://github.com/Good03/pipely.git
cd pipely
go build -o pipely ./cmd/pipely
```

### Option 2: Download Pre-built Binary

Download the latest release from the [Releases](https://github.com/Good03/pipely/releases) page.

### Add to PATH

To run pipely from anywhere in your terminal:

**Windows (PowerShell):**
```powershell
$env:PATH += ";C:\path\to\pipely\directory"
```

**Linux/macOS:**
```bash
export PATH="$PATH:/path/to/pipely/directory"
```

Or copy the executable to a directory already in your PATH.

## Configuration

### Initialize Configuration

On first use, run:

```bash
pipely init
```

This interactive command will:
1. Prompt for your Azure DevOps organization name
2. Prompt for your Personal Access Token (PAT)
3. Display available projects for selection
4. Display available repositories for selection
5. Display available pipelines for selection
6. Save the configuration file on your local machine

### View Configuration

```bash
pipely config
```

Displays your current configuration settings including organization, project, repository, branch, and pipeline IDs.

### Update Configuration

```bash
pipely config set
```

Modify specific configuration values after initial setup.

### Sync Configuration

```bash
pipely config sync
```

Synchronize your local pipeline list with the current Azure DevOps pipelines.

## Usage

### List Pipelines

View all pipelines defined in your configuration:

```bash
pipely pipeline list
```

Output example:
```
Pipelines defined in config:
- ID: 42
- ID: 156
- ID: 203
```

### Run Pipelines

Execute all pipelines defined in your configuration:

```bash
pipely run all
```

This runs all configured pipelines concurrently and displays the results.

Execute all pipelines defined in your configuration:

```bash
pipely run <pipelineId>
```

This runs a pipeline with specific id.

## Configuration File Location

Pipely stores configuration in:

- **Windows**: `%USERPROFILE%\.pipely\config.json`
- **Linux/macOS**: `~/.pipely/config.json`

## Security Notes

 **Important**: Never commit your PAT or configuration file to version control!
- The configuration file contains sensitive information
- Add `.pipely/config.json` to your `.gitignore`
- Rotate your PAT periodically for security
