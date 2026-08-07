package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/RaniduNethma/vedoc/internal/ai"
	"github.com/RaniduNethma/vedoc/internal/generator"
	"github.com/RaniduNethma/vedoc/internal/models"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate API documentation from source code",
	Long:  "Scans the current directory, resolves deterministic Express routes, and generates Postman/Swagger docs.",
	Run: func(cmd *cobra.Command, args []string) {
		var selectedOptions []string
		prompt := &survey.MultiSelect{
			Message: "What documentation formats do you want to generate?",
			Options: []string{"Postman Collection", "Swagger (OpenAPI 3.0)"},
			Default: []string{"Postman Collection", "Swagger (OpenAPI 3.0)"},
		}
		if err := survey.AskOne(prompt, &selectedOptions); err != nil || len(selectedOptions) == 0 {
			fmt.Println("Generation cancelled or no options selected.")
			return
		}

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Prefix = " "
		s.Suffix = " Analyzing project and resolving Express routes..."
		s.Start()

		endpoints, err := analyzeProject(".")
		if err != nil {
			s.Stop()
			fmt.Println("Error analyzing project:", err)
			return
		}
		s.Stop()

		resolved := models.ResolvedEndpoints(endpoints)
		unresolved := len(endpoints) - len(resolved)
		if len(resolved) == 0 {
			if unresolved > 0 {
				fmt.Printf("No statically resolved API endpoints found (%d unresolved; run 'vedoc analyze' for provenance).\n", unresolved)
			} else {
				fmt.Println("No API endpoints found in the current directory.")
			}
			return
		}

		fmt.Printf("Found %d resolved endpoints", len(resolved))
		if unresolved > 0 {
			fmt.Printf(" (%d unresolved and excluded from generated docs)", unresolved)
		}
		fmt.Println(".")

		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Error getting home directory:", err)
			return
		}
		viper.AddConfigPath(homeDir)
		viper.SetConfigName(".vedoc")
		viper.SetConfigType("yaml")
		_ = viper.ReadInConfig()

		apikey := viper.GetString("gemini_api_key")
		if apikey != "" {
			s.Suffix = " AI is generating descriptions and payloads..."
			s.Start()
			enrichedEndpoints, aiErr := ai.EnrichEndpointsBatch(apikey, resolved)
			s.Stop()
			if aiErr == nil {
				resolved = enrichedEndpoints
				fmt.Println("AI analysis completed successfully!")
			} else {
				fmt.Printf("AI API Error: %v (falling back to deterministic docs)\n", aiErr)
			}
		} else {
			fmt.Println("Generating deterministic docs without AI enrichment.")
		}

		fmt.Println("\nGenerating requested files...")
		for _, opt := range selectedOptions {
			switch opt {
			case "Postman Collection":
				if err := generator.GeneratePostmanCollection(resolved, "vedoc_postman_collection.json"); err != nil {
					fmt.Println("Error generating Postman collection:", err)
				} else {
					fmt.Println("Created 'vedoc_postman_collection.json'")
				}
			case "Swagger (OpenAPI 3.0)":
				if err := generator.GenerateSwagger(resolved, "vedoc_swagger.json"); err != nil {
					fmt.Println("Error generating Swagger docs:", err)
				} else {
					fmt.Println("Created 'vedoc_swagger.json'")
				}
			}
		}
		fmt.Println("\nAll done! Documentation is ready to use.")
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
}
