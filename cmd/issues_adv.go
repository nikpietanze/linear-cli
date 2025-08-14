package cmd

import (
	"errors"
	"fmt"
	"strings"

	"linear-cli/internal/api"
	"linear-cli/internal/config"

	"github.com/spf13/cobra"
)

// Enhanced issues commands per requirements (filters, view, create with resolution)

var issuesViewCmd = &cobra.Command{
	Use:   "view <issue-id>",
	Short: "View full details for an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
		client := api.NewClient(cfg.APIKey)
		id := strings.TrimSpace(args[0])
		det, err := client.GetIssueDetails(id)
		if err != nil { return err }
		if det == nil { return fmt.Errorf("issue %s not found", id) }
		p := printer(cmd)
		if p.JSONEnabled() { return p.PrintJSON(det) }
		assignee := ""
		if det.Assignee != nil { assignee = det.Assignee.Name }
		project := ""
		if det.Project != nil { project = det.Project.Name }
		fmt.Printf("%s %s\nState: %s\nAssignee: %s\nProject: %s\nURL: %s\n\n%s\n", det.Identifier, det.Title, det.StateName, assignee, project, det.URL, strings.TrimSpace(det.Description))
		return nil
	},
}

var issuesListAdvCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues with optional filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
		client := api.NewClient(cfg.APIKey)
		limit, _ := cmd.Flags().GetInt("limit")
		project, _ := cmd.Flags().GetString("project")
		assignee, _ := cmd.Flags().GetString("assignee")
		state, _ := cmd.Flags().GetString("state")

		var projectID string
		if project != "" {
			pr, err := client.ResolveProject(project)
			if err != nil { return err }
			if pr == nil { return fmt.Errorf("project '%s' not found", project) }
			projectID = pr.ID
		}
		var assigneeID string
		if assignee != "" {
			u, err := client.ResolveUser(assignee)
			if err != nil { return err }
			if u == nil { return fmt.Errorf("assignee '%s' not found", assignee) }
			assigneeID = u.ID
		}
		items, err := client.ListIssuesFiltered(api.IssueListFilter{ProjectID: projectID, AssigneeID: assigneeID, StateName: state, Limit: limit})
		if err != nil { return err }
		p := printer(cmd)
		if p.JSONEnabled() { return p.PrintJSON(items) }
		head := []string{"Key", "State", "Title"}
		rows := make([][]string, 0, len(items))
		for _, it := range items {
			rows = append(rows, []string{it.Identifier, it.StateName, it.Title})
		}
		return p.Table(head, rows)
	},
}

var issuesCreateAdvCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
		client := api.NewClient(cfg.APIKey)

		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		project, _ := cmd.Flags().GetString("project")
		assignee, _ := cmd.Flags().GetString("assignee")
		label, _ := cmd.Flags().GetString("label")
		priority, _ := cmd.Flags().GetInt("priority")
		if title == "" { return errors.New("--title is required") }
		var projectID string
		if project != "" {
			pr, err := client.ResolveProject(project)
			if err != nil { return err }
			if pr == nil { return fmt.Errorf("project '%s' not found", project) }
			projectID = pr.ID
		}
		var assigneeID string
		if assignee != "" {
			u, err := client.ResolveUser(assignee)
			if err != nil { return err }
			if u == nil { return fmt.Errorf("assignee '%s' not found", assignee) }
			assigneeID = u.ID
		}
		var labelIDs []string
		if label != "" {
			l, err := client.ResolveLabelByName(label)
			if err != nil { return err }
			if l == nil { return fmt.Errorf("label '%s' not found", label) }
			labelIDs = []string{l.ID}
		}
		var prioPtr *int
		if cmd.Flags().Changed("priority") { prioPtr = &priority }
		created, err := client.CreateIssueAdvanced(api.IssueCreateInput{ProjectOrTeamID: projectID, Title: title, Description: description, AssigneeID: assigneeID, LabelIDs: labelIDs, Priority: prioPtr})
		if err != nil { return err }
		p := printer(cmd)
		if p.JSONEnabled() { return p.PrintJSON(created) }
		fmt.Printf("Created %s: %s\n", created.Identifier, created.URL)
		return nil
	},
}

func init() {
	// override list/create with advanced versions and add view
	issuesCmd.RemoveCommand(issuesListCmd)
	issuesCmd.RemoveCommand(issuesCreateCmd)
	issuesCmd.AddCommand(issuesListAdvCmd)
	issuesCmd.AddCommand(issuesViewCmd)
	issuesCmd.AddCommand(issuesCreateAdvCmd)

	issuesListAdvCmd.Flags().Int("limit", 10, "Maximum number of issues to list")
	issuesListAdvCmd.Flags().String("project", "", "Filter by project name or id")
	issuesListAdvCmd.Flags().String("assignee", "", "Filter by assignee name or id")
	issuesListAdvCmd.Flags().String("state", "", "Filter by state name")

	issuesCreateAdvCmd.Flags().String("title", "", "Issue title")
	issuesCreateAdvCmd.Flags().String("description", "", "Issue description")
	issuesCreateAdvCmd.Flags().String("project", "", "Project name or id")
	issuesCreateAdvCmd.Flags().String("assignee", "", "Assignee name or id")
	issuesCreateAdvCmd.Flags().String("label", "", "Label name")
	issuesCreateAdvCmd.Flags().Int("priority", 0, "Priority (1 highest .. 4 lowest)")
}
