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
    Short: "AI-optimized CLI for Linear issue management",
    Long:  "linear-cli is designed for AI agents and automation workflows. Create structured Linear issues with single commands, automatic template discovery, and intelligent caching.",
    Example: `  # 🤖 AI AGENT WORKFLOW (Recommended)
  # 1. Authenticate once
  linear-cli auth login
  
  # 2. Create structured issues instantly
  linear-cli issues create --team ENG --template "Feature Template" --title "Add search" \
    --sections Summary="Implement user search" --sections Context="Users need to find content"
  
  # 3. Discover templates dynamically  
  linear-cli issues template structure --team ENG
  
  # 👤 HUMAN WORKFLOW
  # Interactive creation
  linear-cli issues create --team ENG
  
  # View and manage issues
  linear-cli issues view ENG-123
  linear-cli issues list --project "Website"`,
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
