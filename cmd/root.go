package cmd

import (
    "fmt"
    "os"

    "linear-cli/internal/output"

    "github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "linear-cli",
	Short: "A fast CLI to work with Linear issues",
	Long:  "linear-cli is a fast, plug-and-play CLI to authenticate with Linear and create or read issues via their GraphQL API.",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// Execute runs the root command.
func Execute() {
	// Show friendly suggestions for mistyped commands
	rootCmd.SuggestionsMinimumDistance = 1

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
    // Global flags
    rootCmd.PersistentFlags().BoolP("json", "j", false, "Output JSON for scripting")
}

// helper to access a shared output.Printer from commands
func printer(cmd *cobra.Command) output.Printer {
    jsonOut, _ := cmd.Root().Flags().GetBool("json")
    return output.Printer{JSON: jsonOut}
}
