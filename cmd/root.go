package cmd

import (
    "fmt"
    "os"
    "runtime"
    "strings"

    "linear-cli/internal/output"

    "github.com/spf13/cobra"
)

// These are injected at build time via -ldflags. Defaults are for dev builds.
var (
    buildVersion = "dev"
    buildCommit  = ""
)

var rootCmd = &cobra.Command{
	Use:   "linear-cli",
	Short: "A fast CLI to work with Linear issues",
    Long:  "linear-cli is a fast, plug-and-play CLI to authenticate with Linear and create or read issues via their GraphQL API.",
    Example: `  # Authenticate (stored in ~/.config/linear/config.toml)
  linear-cli auth login

  # Quick auth status (JSON)
  linear-cli --json auth status

  # List issues with filters
  linear-cli issues list --project "Website" --assignee "Jane" --state "In Progress"

  # View an issue by key or ID
  linear-cli issues view ENG-123

  # Create an issue with label and priority
  linear-cli issues create --title "Bug" --label bug --priority 2`,
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
    rootCmd.PersistentFlags().StringP("output", "o", "", "Output format: json|text (alias of --json)")
    rootCmd.MarkFlagsMutuallyExclusive("json", "output")
    // Allow tests to inject a custom API endpoint via env; document via hidden flag if needed later

    // Provide a version flag for packaging (Homebrew requires a simple version output)
    rootCmd.Version = buildVersion
    // Ensure default version flag is initialized so we can set shorthand
    rootCmd.InitDefaultVersionFlag()
    if f := rootCmd.Flags().Lookup("version"); f != nil {
        f.Shorthand = "v" // use -v for version per project preference
        f.Usage = "Show version information"
    }
    // Add an explicit `version` subcommand (e.g., `linear-cli version`)
    rootCmd.AddCommand(&cobra.Command{
        Use:   "version",
        Short: "Show version information",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Cobra doesn't provide a direct way to invoke the version output, so print similarly
            // to the SetVersionTemplate content but using Cobra's version field for consistency.
            fmt.Printf("linear-cli %s\nCommit: %s\nGo: %s\nOS/Arch: %s/%s\n", rootCmd.Version, buildCommit, runtime.Version(), runtime.GOOS, runtime.GOARCH)
            return nil
        },
    })

    // Enrich version output with commit, runtime and platform
    rootCmd.SetVersionTemplate(fmt.Sprintf(`linear-cli %s
Commit: %s
Go: %s
OS/Arch: %s/%s
`, "{{.Version}}", buildCommit, runtime.Version(), runtime.GOOS, runtime.GOARCH))

    // Provide an informative help output with examples and environment info
    rootCmd.SetHelpTemplate(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}{{end}}

Usage:
  {{.UseLine}}

{{if .HasAvailableSubCommands}}Available Commands:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding}} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}
Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}
Additional help topics:
{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}
{{end}}{{end}}{{end}}{{if .Example}}
Examples:
{{.Example}}{{end}}

Environment:
  LINEAR_API_KEY        Linear API key used for authentication
  LINEAR_API_ENDPOINT   Override GraphQL endpoint (testing)

Configuration:
  Config file is stored at ~/.config/linear/config.toml (created by 'auth login').
`)

    // Ensure default help flag exists and set shorthand explicitly
    rootCmd.InitDefaultHelpFlag()
    if f := rootCmd.Flags().Lookup("help"); f != nil {
        f.Shorthand = "h"
        f.Usage = "Show help for command"
    }
    // Ensure a top-level 'help' command is available (e.g., `linear-cli help`)
    rootCmd.InitDefaultHelpCmd()
}

// helper to access a shared output.Printer from commands
func printer(cmd *cobra.Command) output.Printer {
    jsonOut, _ := cmd.Root().Flags().GetBool("json")
    outFmt, _ := cmd.Root().Flags().GetString("output")
    if strings.EqualFold(strings.TrimSpace(outFmt), "json") {
        jsonOut = true
    }
    return output.Printer{JSON: jsonOut}
}
