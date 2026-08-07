package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use: "config",
	Short: "Manage Vedoc Configurations",
	Long: "Set or update your Gemini API key and other configurations globally.",
}

var setKeyCmd = &cobra.Command{
	Use: "set-key [API_KEY]",
	Short: "Set your Gemini api key globally",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := args[0]

		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Error getting home directory:", err)
			return
		}

		configPath := filepath.Join(homeDir, ".vedoc.yml")

		viper.Set("gemini_api_key", apiKey)

		err = viper.WriteConfigAs(configPath)
		if err != nil {
			fmt.Println("Error saving API key: ", err)
			return
		}

		fmt.Printf("Gemini API Key saved successfully to %s\n", configPath)
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(setKeyCmd)
}
