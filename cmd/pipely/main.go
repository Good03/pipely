package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/Good03/pipely/internal/pipely"
	"github.com/spf13/cobra"
)

var debugMode bool

var rootCmd = &cobra.Command{
	Use: "pipely",
}

var runCmd = &cobra.Command{
	Use:     "run",
	Args:    cobra.ExactArgs(1),
	Example: "pipely run <pipelineId>",
	Run: func(cmd *cobra.Command, args []string) {
		path := pipely.GetConfigPath()
		config, _ := pipely.LoadConfig(path)
		pipelineId, _ := strconv.Atoi(args[0])

		level := slog.LevelInfo
		if debugMode {
			level = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
		slog.SetDefault(logger)

		slog.Debug("Checking file existence", "path", path) // Example debug log

		if !pipely.FileExists(path) {

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

			config := pipely.Config{
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
		pipely.RunPipeline(pipelineId, config)
	},
}

var allCmd = &cobra.Command{
	Use:   "all",
	Short: "Runs all pipelines that are defined in config",
	Run: func(cmd *cobra.Command, args []string) {
		path := pipely.GetConfigPath()
		config, _ := pipely.LoadConfig(path)
		pipely.RunPipelines(config)
	},
}

var setCmd = &cobra.Command{
	Use:     "set",
	Short:   "Set a configuration field",
	Example: "pipely config set pipeline_ids \"1,2,3,4,5\"",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		field := args[0]
		value := args[1]
		path := pipely.GetConfigPath()

		if err := pipely.UpdateConfigField(path, field, value); err != nil {
			slog.Error("Failed to update config", "error", err)
			os.Exit(1)
		}
		slog.Info("Successfully updated configuration", "field", field, "value", value)
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug mode")
}

func main() {
	var configCmd = &cobra.Command{
		Use:     "config",
		Short:   "Prints configuration file",
		Aliases: []string{"cfg"},
		Run: func(cmd *cobra.Command, args []string) {
			path := pipely.GetConfigPath()
			_, err := pipely.GetConfig(path)
			if err != nil {
				slog.Error("Failed to read config", "error", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(runCmd)
	configCmd.AddCommand(setCmd)
	runCmd.AddCommand(allCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
