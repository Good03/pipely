package pipely

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Org         string   `json:"org"`
	Project     string   `json:"project"`
	PAT         string   `json:"pat"`
	Repo        string   `json:"repo"`
	Branch      string   `json:"branch"`
	PipelineIds []string `json:"pipeline_ids"`
}

func InitConfig() {
	path := GetConfigPath()
	slog.Debug("Checking file existence", "path", path)

	if FileExists(path) {
		slog.Info("The config file already exists, you can modify it by running \"pipely config set\" command ")
		return
	}

	slog.Info("Config does not exist, creating a config", "path", path)

	var org string
	fmt.Print("Enter your Azure DevOps organisation name: ")
	fmt.Scan(&org)

	var pat string
	fmt.Print("Enter Azure DevOps your PAT: ")
	fmt.Scan(&pat)

	// Fetch projects
	projects, err := FetchProjects(org, pat)
	if err != nil {
		slog.Error("Failed to fetch projects", "error", err)
		os.Exit(1)
	}
	selectedProject := SelectProject(projects)

	// Fetch repos
	repos, err := FetchRepos(org, selectedProject.Name, pat)
	if err != nil {
		slog.Error("Failed to fetch repos", "error", err)
		os.Exit(1)
	}
	selectedRepo := SelectRepo(repos)

	// Fetch pipelines
	pipelines, err := FetchPipelines(org, selectedProject.Name, pat)
	if err != nil {
		slog.Error("Failed to fetch pipelines", "error", err)
		os.Exit(1)
	}
	selectedPipelines := SelectPipelines(pipelines)

	var pipelineIds []string
	for _, p := range selectedPipelines {
		pipelineIds = append(pipelineIds, p.Id)
	}

	config := Config{
		Org:         org,
		Project:     selectedProject.Name,
		PAT:         pat,
		Repo:        selectedRepo.Name,
		Branch:      "main",
		PipelineIds: pipelineIds,
	}

	configJson, _ := json.MarshalIndent(config, "", "  ")

	err = os.WriteFile(path, configJson, 0644)
	if err != nil {
		slog.Error("Failed to write config", "error", err)
		os.Exit(1)
	}
	slog.Info("Successfully created config", "path", path)
}

func LoadConfig(path string) (Config, error) {
	dataFromFile, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	config := Config{}
	err = json.Unmarshal(dataFromFile, &config)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func PrintConfig() {
	path := GetConfigPath()
	config, err := LoadConfig(path)
	if err != nil {
		slog.Error("Error while reading config", "error", err)
	}

	fmt.Printf("Org: %s\n", config.Org)
	fmt.Printf("Project: %s\n", config.Project)
	fmt.Printf("Repo: %s\n", config.Repo)
	fmt.Printf("Branch: %s\n", config.Branch)
	fmt.Printf("Pipeline Ids: %v", config.PipelineIds)
}

func ListPipelines(path string) error {
	config, err := LoadConfig(path)
	if err != nil {
		return err
	}

	fmt.Println("Pipelines defined in config:")
	for _, id := range config.PipelineIds {
		fmt.Printf("- ID: %s\n", id)
	}
	return nil
}

func SyncConfig(path string) error {
	config, err := LoadConfig(path)
	if err != nil {
		return err
	}

	pipelines, err := FetchPipelines(config.Org, config.Project, config.PAT)
	if err != nil {
		return err
	}

	selected := SelectPipelines(pipelines)
	var newIds []string
	for _, p := range selected {
		newIds = append(newIds, p.Id)
	}
	config.PipelineIds = newIds

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func UpdateConfigField(path string, field string, value string) error {
	config, err := LoadConfig(path)
	if err != nil {
		return err
	}

	switch field {
	case "org":
		config.Org = value
	case "project":
		config.Project = value
	case "pat":
		config.PAT = value
	case "repo":
		config.Repo = value
	case "branch":
		config.Branch = value
	case "pipeline_ids":
		parsedIds := strings.Split(value, ",")
		var ids []string

		for _, id := range parsedIds {
			ids = append(ids, id)
		}

		config.PipelineIds = ids
	default:
		return fmt.Errorf("unknown field: %s", field)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func GetConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		slog.Error("Could not determine user config directory", "error", err)
		os.Exit(1)
	}

	appConfigDir := filepath.Join(configDir, "pipely")

	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		slog.Error("Failed to create config directory", "error", err)
		os.Exit(1)
	}

	return filepath.Join(appConfigDir, "config.json")
}
