package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/Good03/pipely/internal/pipely"
	"github.com/spf13/cobra"
)

var debugMode bool
var configPath = pipely.GetConfigPath()
var showConfigPath bool

var rootCmd = &cobra.Command{
	Use: "pipely",
	Run: func(cmd *cobra.Command, args []string) {
		if showConfigPath {
			fmt.Println(configPath)
			return
		}
	},
}

var runCmd = &cobra.Command{
	Use:     "run",
	Args:    cobra.ExactArgs(1),
	Example: "pipely run <pipelineId>",
	Run: func(cmd *cobra.Command, args []string) {
		path := configPath

		config, _ := pipely.LoadConfig(path)
		pipelineID, err := strconv.Atoi(args[0])
		if err != nil {
			slog.Error("Pipeline ID must be an integer", "value", args[0], "error", err)
			os.Exit(1)
		}

		level := slog.LevelInfo
		if debugMode {
			level = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
		slog.SetDefault(logger)

		pipely.RunPipeline(pipelineID, config)
	},
}

var allCmd = &cobra.Command{
	Use:   "all",
	Short: "Runs all pipelines that are defined in config",
	Run: func(cmd *cobra.Command, args []string) {
		path := configPath
		config, _ := pipely.LoadConfig(path)
		pipely.RunPipelines(config)
	},
}

var initConfigCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize config file",
	Run: func(cmd *cobra.Command, args []string) {
		pipely.InitConfig()
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
		path := configPath

		if err := pipely.UpdateConfigField(path, field, value); err != nil {
			slog.Error("Failed to update config", "error", err)
			os.Exit(1)
		}
		slog.Info("Successfully updated configuration", "field", field, "value", value)
	},
}

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Prints configuration file",
	Aliases: []string{"cfg"},
	Run: func(cmd *cobra.Command, args []string) {
		pipely.PrintConfig()
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync pipelines between config and ADO",
	Run: func(cmd *cobra.Command, args []string) {
		path := configPath
		if err := pipely.SyncConfig(path); err != nil {
			slog.Error("Failed to sync config", "error", err)
			os.Exit(1)
		}
		slog.Info("Successfully synced configuration")
	},
}

var pipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Manage pipelines",
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List pipelines from config",
	Run: func(cmd *cobra.Command, args []string) {
		path := configPath
		if err := pipely.ListPipelines(path); err != nil {
			slog.Error("Failed to list pipelines", "error", err)
			os.Exit(1)
		}
	},
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Run pipely TUI",
	Run: func(cmd *cobra.Command, args []string) {
		path := configPath
		config, _ := pipely.LoadConfig(path)
		pipely.RunTui(config)
	},
}

func init() {
	rootCmd.Flags().BoolVar(&showConfigPath, "config-path", false, "Print the config file location")
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug mode")
}
