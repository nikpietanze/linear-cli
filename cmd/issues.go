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

var issuesCmd = &cobra.Command{
	Use:   "issues",
	Short: "Work with Linear issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var issuesListCmd = &cobra.Command{
    Use:   "list",
    Short: "List recent issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		apiKey := cfg.APIKey
		if apiKey == "" {
			return errors.New("not authenticated. run 'linear-cli auth login'")
		}
		client := api.NewClient(apiKey)

		limit, _ := cmd.Flags().GetInt("limit")
		teamKey, _ := cmd.Flags().GetString("team")

		var teamID string
		if teamKey != "" {
			team, err := client.TeamByKey(teamKey)
			if err != nil {
				return err
			}
			if team == nil {
				return fmt.Errorf("team with key %s not found", teamKey)
			}
			teamID = team.ID
		}

		issues, err := client.ListIssues(limit, teamID)
		if err != nil {
			return err
		}

        // default simple output retained for compatibility; advanced list replaces this in issues_adv.go
        for _, is := range issues {
            fmt.Printf("%s\t[%s]\t%s\n", is.Identifier, is.StateName, is.Title)
        }
        return nil
	},
}

var issuesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a single issue by id or key (TEAM-123)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		apiKey := cfg.APIKey
		if apiKey == "" {
			return errors.New("not authenticated. run 'linear-cli auth login'")
		}
		client := api.NewClient(apiKey)

		id, _ := cmd.Flags().GetString("id")
		key, _ := cmd.Flags().GetString("key")
		if id == "" && key == "" {
			return errors.New("provide either --id or --key TEAM-123")
		}

		var issue *api.Issue
		var err error
		if id != "" {
			issue, err = client.IssueByID(id)
		} else {
			key = strings.ToUpper(key)
			re := regexp.MustCompile(`^([A-Z]+)-(\d+)$`)
			m := re.FindStringSubmatch(key)
			if len(m) != 3 {
				return errors.New("--key must be in format TEAM-123")
			}
			teamKey := m[1]
			num, _ := strconv.Atoi(m[2])
			team, errT := client.TeamByKey(teamKey)
			if errT != nil {
				return errT
			}
			if team == nil {
				return fmt.Errorf("team with key %s not found", teamKey)
			}
			issue, err = client.IssueByKey(team.ID, num)
		}
		if err != nil {
			return err
		}
		if issue == nil {
			fmt.Println("Issue not found")
			return nil
		}
        fmt.Printf("%s %s\nState: %s\nURL: %s\n\n%s\n", issue.Identifier, issue.Title, issue.StateName, issue.URL, strings.TrimSpace(issue.Description))
		return nil
	},
}

var issuesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		apiKey := cfg.APIKey
		if apiKey == "" {
			return errors.New("not authenticated. run 'linear-cli auth login'")
		}
		client := api.NewClient(apiKey)

		teamKey, _ := cmd.Flags().GetString("team")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		if teamKey == "" || title == "" {
			return errors.New("--team and --title are required")
		}
		team, err := client.TeamByKey(teamKey)
		if err != nil {
			return err
		}
		if team == nil {
			return fmt.Errorf("team with key %s not found", teamKey)
		}

		issue, err := client.CreateIssue(team.ID, title, description)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s: %s\n", issue.Identifier, issue.URL)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(issuesCmd)
	issuesCmd.AddCommand(issuesListCmd)
	issuesCmd.AddCommand(issuesGetCmd)
	issuesCmd.AddCommand(issuesCreateCmd)

    issuesListCmd.Flags().IntP("limit", "n", 10, "Maximum number of issues to list")
    issuesListCmd.Flags().StringP("team", "t", "", "Filter by team key (e.g. ENG)")

    issuesGetCmd.Flags().StringP("id", "i", "", "Issue ID")
    issuesGetCmd.Flags().StringP("key", "k", "", "Issue key like TEAM-123")

    issuesCreateCmd.Flags().StringP("team", "t", "", "Team key (e.g. ENG)")
    issuesCreateCmd.Flags().StringP("title", "T", "", "Issue title")
    issuesCreateCmd.Flags().StringP("description", "d", "", "Issue description")
}
