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

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Write a comment on an issue",
	RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

var commentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a comment on an issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
		client := api.NewClient(cfg.APIKey)

		issueID, _ := cmd.Flags().GetString("id")
		issueKey, _ := cmd.Flags().GetString("key")
		body, _ := cmd.Flags().GetString("body")
		if body == "" { return errors.New("--body is required") }
		if issueID == "" && issueKey == "" { return errors.New("provide --id or --key TEAM-123") }

		if issueID == "" {
			// Resolve TEAM-123
			key := strings.ToUpper(strings.TrimSpace(issueKey))
			re := regexp.MustCompile(`^([A-Z]+)-(\d+)$`)
			m := re.FindStringSubmatch(key)
			if len(m) != 3 { return errors.New("--key must be TEAM-123 format") }
			teamKey := m[1]
			n, _ := strconv.Atoi(m[2])
			team, err := client.TeamByKey(teamKey)
			if err != nil { return err }
			if team == nil { return fmt.Errorf("team with key %s not found", teamKey) }
			iss, err := client.IssueByKey(team.ID, n)
			if err != nil { return err }
			if iss == nil { return fmt.Errorf("issue %s not found", key) }
			issueID = iss.ID
		}

		res, err := client.CreateComment(issueID, body)
		if err != nil { return err }
		p := printer(cmd)
		if p.JSONEnabled() {
			return p.PrintJSON(res)
		}
		fmt.Printf("Comment %s created on %s: %s\n", res.Comment.ID, res.IssueKey, res.IssueURL)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(commentCmd)
	commentCmd.AddCommand(commentCreateCmd)
    commentCreateCmd.Flags().StringP("id", "i", "", "Issue ID")
    commentCreateCmd.Flags().StringP("key", "k", "", "Issue key like TEAM-123")
    commentCreateCmd.Flags().StringP("body", "b", "", "Comment body (markdown supported)")
}
