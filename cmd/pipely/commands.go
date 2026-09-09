package main

import (
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
		path := pipely.GetConfigPath()

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
		path := pipely.GetConfigPath()
		_, err := pipely.LoadConfig(path)
		if err != nil {
			slog.Error("Failed to read config", "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug mode")
}
