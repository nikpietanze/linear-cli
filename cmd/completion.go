package cmd

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

// completionCmd provides shell completion scripts for popular shells.
var completionCmd = &cobra.Command{
    Use:   "completion [bash|zsh|fish|powershell]",
    Short: "Generate shell completion script",
    Long: `To enable completion run the following or add to your shell profile:

Bash:
  source <(linear-cli completion bash)
  # or write to a file:
  linear-cli completion bash > /usr/local/etc/bash_completion.d/linear-cli

Zsh:
  linear-cli completion zsh > "${fpath[1]}/_linear-cli"
  autoload -U compinit && compinit

Fish:
  linear-cli completion fish | source
  # or:
  linear-cli completion fish > ~/.config/fish/completions/linear-cli.fish

PowerShell:
  linear-cli completion powershell | Out-String | Invoke-Expression
`,
    Args: cobra.ExactArgs(1),
    ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
    RunE: func(cmd *cobra.Command, args []string) error {
        switch args[0] {
        case "bash":
            return rootCmd.GenBashCompletion(os.Stdout)
        case "zsh":
            return rootCmd.GenZshCompletion(os.Stdout)
        case "fish":
            return rootCmd.GenFishCompletion(os.Stdout, true)
        case "powershell":
            return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
        default:
            return fmt.Errorf("unknown shell: %s", args[0])
        }
    },
}

func init() {
    rootCmd.AddCommand(completionCmd)
}
