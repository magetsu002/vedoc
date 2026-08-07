package cmd

import (
	"fmt"

	"github.com/RaniduNethma/vedoc/internal/contract"
	"github.com/spf13/cobra"
)

var analyzeOutput string

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Write a deterministic API contract snapshot",
	Long:  "Analyze the current project without AI and write Vedoc's deterministic endpoint IR, including resolution state and source provenance.",
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoints, err := analyzeProject(".")
		if err != nil {
			return fmt.Errorf("analyze project: %w", err)
		}

		snapshot := contract.BuildSnapshot(endpoints)
		if err := contract.WriteSnapshot(analyzeOutput, snapshot); err != nil {
			return fmt.Errorf("write contract snapshot: %w", err)
		}

		resolved := 0
		unresolved := 0
		for _, endpoint := range snapshot.Endpoints {
			if endpoint.IsResolved() && endpoint.Path != "" {
				resolved++
			} else {
				unresolved++
			}
		}
		fmt.Printf("Wrote %s (%d resolved, %d unresolved)\n", analyzeOutput, resolved, unresolved)
		return nil
	},
}

func init() {
	analyzeCmd.Flags().StringVarP(&analyzeOutput, "output", "o", "vedoc_contract.json", "output path for the deterministic contract snapshot")
	rootCmd.AddCommand(analyzeCmd)
}
