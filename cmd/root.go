package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
  )
  

var rootCmd = &cobra.Command{
	Use:   "delta-share",
	Short: "delta-share returns a Databricks Delta Share URL for you to use to gain access to shared data within Databricks",
	Run: func(cmd *cobra.Command, args []string) {
	  // Do Stuff Here
	  fmt.Println("Hello, World!")
	},
}
  
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}