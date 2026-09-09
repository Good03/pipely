package main

import (
	"fmt"
	"os"
)

func main() {
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(initConfigCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(pipelineCmd)
	configCmd.AddCommand(setCmd)
	configCmd.AddCommand(syncCmd)
	runCmd.AddCommand(allCmd)
	pipelineCmd.AddCommand(listCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
