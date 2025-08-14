package cmd

import (
    "errors"
    "fmt"

    "linear-cli/internal/api"
    "linear-cli/internal/config"

    "github.com/spf13/cobra"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Work with Linear projects",
	RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

var projectsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
		client := api.NewClient(cfg.APIKey)
    ps, err := client.ListProjects()
    if err != nil {
        if printer(cmd).JSONEnabled() {
            printer(cmd).PrintError(err)
            return err
        }
        return fmt.Errorf("failed to list projects. Ensure your Linear API key has read access to projects. Original error: %w", err)
    }
		p := printer(cmd)
		if p.JSONEnabled() {
			return p.PrintJSON(ps)
		}
    head := []string{"ID", "Name"}
		rows := make([][]string, 0, len(ps))
		for _, pr := range ps {
        rows = append(rows, []string{pr.ID, pr.Name})
		}
		return p.Table(head, rows)
	},
}

func init() {
	rootCmd.AddCommand(projectsCmd)
	projectsCmd.AddCommand(projectsListCmd)
}
