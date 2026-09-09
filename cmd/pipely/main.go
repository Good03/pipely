package main

import (
	"fmt"
	"os"
)

func main() {
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(initConfigCmd)
	rootCmd.AddCommand(runCmd)
	configCmd.AddCommand(setCmd)
	runCmd.AddCommand(allCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
