package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use: "vedoc",
	Short: "Instant API Docs. No Comments Required.",
	Long: "Vedoc scans your codebase, parses the AST using Tree-sitter, and uses AI to automatically generate Postman/Swagger documentation.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print("Welcome to Vedoc, Run 'vedoc --help' to get started.")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
