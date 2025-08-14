package cmd

import (
    "errors"

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
		if err != nil { return err }
		p := printer(cmd)
		if p.JSONEnabled() {
			return p.PrintJSON(ps)
		}
		head := []string{"ID", "Name", "State"}
		rows := make([][]string, 0, len(ps))
		for _, pr := range ps {
			rows = append(rows, []string{pr.ID, pr.Name, pr.State})
		}
		return p.Table(head, rows)
	},
}

func init() {
	rootCmd.AddCommand(projectsCmd)
	projectsCmd.AddCommand(projectsListCmd)
}
