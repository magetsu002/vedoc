package cmd

import (
	"fmt"

	"github.com/RaniduNethma/vedoc/internal/contract"
	"github.com/spf13/cobra"
)

var failOnBreaking bool

var diffCmd = &cobra.Command{
	Use:   "diff <before.json> <after.json>",
	Short: "Compare deterministic API contract snapshots",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		before, err := contract.ReadSnapshot(args[0])
		if err != nil {
			return fmt.Errorf("read baseline snapshot: %w", err)
		}
		after, err := contract.ReadSnapshot(args[1])
		if err != nil {
			return fmt.Errorf("read current snapshot: %w", err)
		}

		result := contract.Diff(before, after)
		for _, endpoint := range result.Removed {
			fmt.Printf("BREAKING removed %s %s\n", endpoint.Method, endpoint.Path)
		}
		for _, endpoint := range result.Added {
			fmt.Printf("ADDED %s %s\n", endpoint.Method, endpoint.Path)
		}
		if result.IgnoredUnresolvedBefore > 0 || result.IgnoredUnresolvedAfter > 0 {
			fmt.Printf("Ignored unresolved endpoints: before=%d after=%d\n", result.IgnoredUnresolvedBefore, result.IgnoredUnresolvedAfter)
		}
		if len(result.Removed) == 0 && len(result.Added) == 0 {
			fmt.Println("No resolved API contract changes.")
		}
		if failOnBreaking && result.Breaking {
			return fmt.Errorf("breaking API contract changes detected")
		}
		return nil
	},
}

func init() {
	diffCmd.Flags().BoolVar(&failOnBreaking, "fail-on-breaking", false, "return a non-zero exit status when resolved endpoints are removed")
	rootCmd.AddCommand(diffCmd)
}
