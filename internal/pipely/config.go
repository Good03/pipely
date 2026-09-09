package pipely

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Org         string `json:"org"`
	Project     string `json:"project"`
	PAT         string `json:"pat"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	PipelineIds []int  `json:"pipeline_ids"`
}

func InitConfig() {
	path := GetConfigPath()
	slog.Debug("Checking file existence", "path", path) // Example debug log

	if !FileExists(path) {

		slog.Info("Config does not exist, creating a config", "path", path)

		var org string
		fmt.Print("Enter your Azure DevOps organisation name: ")
		fmt.Scan(&org)

		var project string
		fmt.Print("Enter your Azure DevOps Project name: ")
		fmt.Scan(&project)

		var pat string
		fmt.Print("Enter Azure DevOps your PAT: ")
		fmt.Scan(&pat)

		config := Config{
			Org:         org,
			Project:     project,
			PipelineIds: []int{73768, 73778},
			PAT:         pat,
			Repo:        "org",
			Branch:      "test/pipely",
		}
		configJson, _ := json.MarshalIndent(config, "", "  ")

		err := os.WriteFile(path, configJson, 0644)
		if err != nil {
			slog.Error("Failed to write config", "error", err)
			os.Exit(1)
		}
	}
	slog.Info("The config file already exists, you can modify it by running \"pipely config set\" command ")
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
		var numericIds []int
		parsedIds := strings.Split(value, ",")

		for _, id := range parsedIds {
			numericId, _ := strconv.Atoi(id)
			numericIds = append(numericIds, numericId)
		}

		config.PipelineIds = numericIds
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
