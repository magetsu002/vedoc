package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/RaniduNethma/vedoc/internal/ai"
	"github.com/RaniduNethma/vedoc/internal/generator"
	"github.com/RaniduNethma/vedoc/internal/models"
	"github.com/RaniduNethma/vedoc/internal/parser"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate API documentation from source code",
	Long:  "Scans the current directory, parses code using Tree-sitter, and generates Postman/Swagger docs.",
	Run: func(cmd *cobra.Command, args []string) {

		var selectedOptions []string
		prompt := &survey.MultiSelect{
			Message: "What documentation formats do you want to generate?",
			Options: []string{"Postman Collection", "Swagger (OpenAPI 3.0)"},
			Default: []string{"Postman Collection", "Swagger (OpenAPI 3.0)"},
		}
		err := survey.AskOne(prompt, &selectedOptions)
		if err != nil || len(selectedOptions) == 0 {
			fmt.Println("Generation cancelled or no options selected.")
			return
		}

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Prefix = " "
		s.Suffix = " Scanning codebase for API routes..."
		s.Start()

		var sourceFiles []parser.SourceFile
		err = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if d.Name() == "node_modules" || (strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(d.Name(), ".js") && !strings.HasSuffix(d.Name(), ".ts") {
				return nil
			}

			sourceCode, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
		sourceFiles = append(sourceFiles, parser.SourceFile{Path: path, Source: sourceCode})
			return nil
		})

		if err != nil {
			s.Stop()
			fmt.Println("Error scanning directory:", err)
			return
		}

		endpoints, err := parser.ResolveExpressProject(sourceFiles)
		if err != nil {
			s.Stop()
			fmt.Println("Error resolving Express routes:", err)
			return
		}

		s.Stop()

		if len(endpoints) == 0 {
			fmt.Println("No API endpoints found in the current directory.")
			return
		}

		fmt.Printf("Found %d endpoints across the project!\n", len(endpoints))

		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Error getting home directory: ", err)
			return
		}

		viper.AddConfigPath(homeDir)
		viper.SetConfigName(".vedoc")
		viper.SetConfigType("yaml")

		if err := viper.ReadInConfig(); err != nil {
		}

		apikey := viper.GetString("gemini_api_key")

		if apikey != "" {
			s.Suffix = " AI is generating descriptions and payloads..."
			s.Start()

			enrichedEndpoints, aiErr := ai.EnrichEndpointsBatch(apikey, endpoints)
			s.Stop()

			if aiErr == nil {
				endpoints = enrichedEndpoints
				fmt.Println("AI analysis completed successfully!")
			} else {
				fmt.Printf("AI API Error: %v (Falling back to basic docs)\n", aiErr)
			}
		} else {
			fmt.Println("Generating basic docs... (Run 'vedoc config set-key <KEY>' for AI features)")
		}

		fmt.Println("\n Generating requested files...")

		for _, opt := range selectedOptions {
			if opt == "Postman Collection" {
				err = generator.GeneratePostmanCollection(endpoints, "vedoc_postman_collection.json")
				if err != nil {
					fmt.Println("Error generating Postman collection: ", err)
				} else {
					fmt.Println("Created 'vedoc_postman_collection.json'")
				}
			} else if opt == "Swagger (OpenAPI 3.0)" {
				err = generator.GenerateSwagger(endpoints, "vedoc_swagger.json")
				if err != nil {
					fmt.Println("Error generating Swagger docs: ", err)
				} else {
					fmt.Println("Created 'vedoc_swagger.json'")
				}
			}
		}

		fmt.Println("\n All done! Documentation is ready to use.")
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
}
