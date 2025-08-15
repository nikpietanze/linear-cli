package cmd

import (
	"errors"
	"fmt"
    "regexp"
    "strconv"
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
        raw := strings.TrimSpace(args[0])
        comments, _ := cmd.Flags().GetInt("comments")
        var det *api.IssueDetails
        var err error
        // Accept either an issue ID or a key like TEAM-123
        id := raw
        if m := regexp.MustCompile(`^([A-Z]+)-(\d+)$`).FindStringSubmatch(strings.ToUpper(raw)); len(m) == 3 {
            // Resolve by team+number
            teamKey := m[1]
            num, _ := strconv.Atoi(m[2])
            team, errT := client.TeamByKey(teamKey)
            if errT != nil { return errT }
            if team == nil { return fmt.Errorf("team with key %s not found", teamKey) }
            iss, errK := client.IssueByKey(team.ID, num)
            if errK != nil { return errK }
            if iss == nil { return fmt.Errorf("issue %s not found", raw) }
            id = iss.ID
        }
        if comments > 0 { det, err = client.GetIssueDetailsWithComments(id, comments) } else { det, err = client.GetIssueDetails(id) }
		if err != nil { return err }
		if det == nil { return fmt.Errorf("issue %s not found", id) }
		p := printer(cmd)
		if p.JSONEnabled() { return p.PrintJSON(det) }
		assignee := ""
		if det.Assignee != nil { assignee = det.Assignee.Name }
		project := ""
		if det.Project != nil { project = det.Project.Name }
        fmt.Printf("%s %s\nState: %s\nAssignee: %s\nProject: %s\nURL: %s\n\n%s\n", det.Identifier, det.Title, det.StateName, assignee, project, det.URL, strings.TrimSpace(det.Description))
        if comments > 0 && len(det.Comments) > 0 {
            fmt.Println("\nComments:")
            for _, c := range det.Comments { fmt.Printf("- %s\n", strings.TrimSpace(c.Body)) }
        }
		return nil
	},
}

func runIssuesListWithArgs(cmd *cobra.Command, statePreset string) error {
    cfg, _ := config.Load()
    if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
    client := api.NewClient(cfg.APIKey)
    limit, _ := cmd.Flags().GetInt("limit")
    project, _ := cmd.Flags().GetString("project")
    assignee, _ := cmd.Flags().GetString("assignee")
    stateFlag, _ := cmd.Flags().GetString("state")
    // Convenience boolean flags
    todo, _ := cmd.Flags().GetBool("todo")
    doing, _ := cmd.Flags().GetBool("doing")
    done, _ := cmd.Flags().GetBool("done")

    // Determine effective state
    var state string
    if statePreset != "" { state = statePreset }
    count := 0
    if todo { state = "Todo"; count++ }
    if doing { state = "In Progress"; count++ }
    if done { state = "Done"; count++ }
    if count > 1 { return errors.New("use only one of --todo/--doing/--done") }
    if state == "" { state = normalizeState(stateFlag) }

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
}

func normalizeState(s string) string {
    if s == "" { return "" }
    ls := strings.ToLower(strings.TrimSpace(s))
    switch ls {
    case "todo", "to-do":
        return "Todo"
    case "doing", "inprogress", "in-progress", "in progress":
        return "In Progress"
    case "done", "complete", "completed":
        return "Done"
    default:
        // return original to allow custom states
        return s
    }
}

var issuesListAdvCmd = &cobra.Command{
    Use:   "list",
    Short: "List issues with optional filters",
    RunE: func(cmd *cobra.Command, args []string) error { return runIssuesListWithArgs(cmd, "") },
}

var issuesTodoCmd = &cobra.Command{
    Use:   "todo",
    Short: "List Todo issues",
    RunE: func(cmd *cobra.Command, args []string) error { return runIssuesListWithArgs(cmd, "Todo") },
}

var issuesDoingCmd = &cobra.Command{
    Use:   "doing",
    Short: "List In Progress issues",
    RunE: func(cmd *cobra.Command, args []string) error { return runIssuesListWithArgs(cmd, "In Progress") },
}

var issuesDoneCmd = &cobra.Command{
    Use:   "done",
    Short: "List Done issues",
    RunE: func(cmd *cobra.Command, args []string) error { return runIssuesListWithArgs(cmd, "Done") },
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
    issuesCmd.AddCommand(issuesTodoCmd)
    issuesCmd.AddCommand(issuesDoingCmd)
    issuesCmd.AddCommand(issuesDoneCmd)

    issuesListAdvCmd.Flags().Int("limit", 10, "Maximum number of issues to list")
    issuesListAdvCmd.Flags().String("project", "", "Filter by project name or id")
    issuesListAdvCmd.Flags().String("assignee", "", "Filter by assignee name or id")
    issuesListAdvCmd.Flags().StringP("state", "s", "", "Filter by state (e.g. Todo, In Progress, Done)")
    issuesListAdvCmd.Flags().Bool("todo", false, "Shortcut for --state 'Todo'")
    issuesListAdvCmd.Flags().Bool("doing", false, "Shortcut for --state 'In Progress'")
    issuesListAdvCmd.Flags().Bool("done", false, "Shortcut for --state 'Done'")

    // Reuse common flags for state subcommands
    for _, c := range []*cobra.Command{issuesTodoCmd, issuesDoingCmd, issuesDoneCmd} {
        c.Flags().Int("limit", 10, "Maximum number of issues to list")
        c.Flags().String("project", "", "Filter by project name or id")
        c.Flags().String("assignee", "", "Filter by assignee name or id")
    }

	issuesCreateAdvCmd.Flags().String("title", "", "Issue title")
	issuesCreateAdvCmd.Flags().String("description", "", "Issue description")
	issuesCreateAdvCmd.Flags().String("project", "", "Project name or id")
	issuesCreateAdvCmd.Flags().String("assignee", "", "Assignee name or id")
	issuesCreateAdvCmd.Flags().String("label", "", "Label name")
	issuesCreateAdvCmd.Flags().Int("priority", 0, "Priority (1 highest .. 4 lowest)")
    issuesViewCmd.Flags().Int("comments", 0, "Include up to N comments")
}
