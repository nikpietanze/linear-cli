package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"linear-cli/internal/api"
	"linear-cli/internal/config"

	"github.com/spf13/cobra"
)

// TemplateMetadata stores information about synced templates
type TemplateMetadata struct {
	Templates map[string]TeamTemplates `json:"templates"`
	LastSync  time.Time                `json:"last_sync"`
}

type TeamTemplates struct {
	TeamID    string                    `json:"team_id"`
	TeamKey   string                    `json:"team_key"`
	Templates map[string]TemplateInfo   `json:"templates"`
	LastSync  time.Time                 `json:"last_sync"`
}

type TemplateInfo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Filename      string    `json:"filename"`
	LastSync      time.Time `json:"last_sync"`
	Description   string    `json:"description,omitempty"`
	RefIssueID    string    `json:"ref_issue_id,omitempty"`
	RefIssueKey   string    `json:"ref_issue_key,omitempty"`
}

type SyncResult struct {
	SkipReason   string
	SyncSummary  string
	NewTemplates int
	UpdatedTemplates int
	RemovedTemplates int
}

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Manage issue templates",
	Long: `Manage issue templates with local caching and server-side synchronization.

Templates are synced from Linear's API and stored locally for fast access during issue creation.
The CLI uses local templates for interactive prompts but still applies templates server-side
for consistency with Linear's web interface.

Commands:
  sync     Sync templates from Linear API to local storage
  list     List locally cached templates
  show     Show a specific template's content
  status   Show sync status for teams`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var templatesSyncCmd = &cobra.Command{
	Use:   "sync [--team <key>] [--all]",
	Short: "Sync templates from Linear API to local storage",
	Long: `Sync issue templates from Linear's API to local storage for fast access.

This command intelligently syncs templates by:
- Detecting new templates that need to be cached
- Identifying templates that have changed structure
- Reusing existing reference issues when possible
- Only creating new reference issues when necessary

Reference issues are clearly labeled with [TEMPLATE-REF] prefix and serve as permanent examples.

Examples:
  linear-cli templates sync --team POK    # Sync templates for team POK
  linear-cli templates sync --all         # Sync templates for all accessible teams`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" {
			return errors.New("not authenticated. run 'linear-cli auth login'")
		}

		client := api.NewClient(cfg.APIKey)
		teamKey, _ := cmd.Flags().GetString("team")
		syncAll, _ := cmd.Flags().GetBool("all")

		if !syncAll && strings.TrimSpace(teamKey) == "" {
			return errors.New("either --team <key> or --all is required")
		}

		templatesDir, err := getTemplatesDir()
		if err != nil {
			return fmt.Errorf("failed to create templates directory: %w", err)
		}

		metadata, err := loadTemplateMetadata(templatesDir)
		if err != nil {
			// Create new metadata if it doesn't exist
			metadata = &TemplateMetadata{
				Templates: make(map[string]TeamTemplates),
			}
		}

		var teamsToSync []api.Team

		if syncAll {
			// Get all teams the user has access to
			teams, err := client.ListTeams()
			if err != nil {
				return fmt.Errorf("failed to list teams: %w", err)
			}
			teamsToSync = teams
		} else {
			// Sync specific team
			team, err := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
			if err != nil {
				return fmt.Errorf("failed to find team %s: %w", teamKey, err)
			}
			if team == nil {
				return fmt.Errorf("team with key %s not found", teamKey)
			}
			teamsToSync = []api.Team{*team}
		}

		for _, team := range teamsToSync {
			fmt.Printf("Checking templates for team %s (%s)...\n", team.Key, team.Name)
			
			syncResult, err := syncTeamTemplatesIntelligent(client, team, templatesDir, metadata)
			if err != nil {
				fmt.Printf("  Error syncing %s: %v\n", team.Key, err)
				continue
			}
			
			if syncResult.SkipReason != "" {
				fmt.Printf("  %s: %s\n", team.Key, syncResult.SkipReason)
			} else {
				fmt.Printf("  %s: %s\n", team.Key, syncResult.SyncSummary)
			}
		}

		// Save updated metadata
		metadata.LastSync = time.Now()
		err = saveTemplateMetadata(templatesDir, metadata)
		if err != nil {
			return fmt.Errorf("failed to save metadata: %w", err)
		}

		fmt.Println("Template sync completed!")
		return nil
	},
}

var templatesListCmd = &cobra.Command{
	Use:   "list [--team <key>]",
	Short: "List locally cached templates",
	Long: `List templates that have been synced locally.

Without --team: Lists all cached templates grouped by team
With --team: Lists templates for a specific team`,
	RunE: func(cmd *cobra.Command, args []string) error {
		templatesDir, err := getTemplatesDir()
		if err != nil {
			return fmt.Errorf("failed to access templates directory: %w", err)
		}

		metadata, err := loadTemplateMetadata(templatesDir)
		if err != nil {
			return fmt.Errorf("no templates found. Run 'linear-cli templates sync' first")
		}

		teamKey, _ := cmd.Flags().GetString("team")
		p := printer(cmd)

		if strings.TrimSpace(teamKey) != "" {
			// List templates for specific team
			teamKey = strings.ToUpper(strings.TrimSpace(teamKey))
			teamData, exists := metadata.Templates[teamKey]
			if !exists {
				return fmt.Errorf("no templates found for team %s. Run 'linear-cli templates sync --team %s' first", teamKey, teamKey)
			}

			templateNames := make([]string, 0, len(teamData.Templates))
			for _, template := range teamData.Templates {
				templateNames = append(templateNames, template.Name)
			}

			if p.JSONEnabled() {
				return p.PrintJSON(map[string]interface{}{
					"team":      teamKey,
					"templates": templateNames,
					"last_sync": teamData.LastSync,
				})
			}

			fmt.Printf("Templates for team %s (synced %v ago):\n", teamKey, time.Since(teamData.LastSync).Round(time.Minute))
			for _, name := range templateNames {
				fmt.Printf("  - %s\n", name)
			}
			return nil
		}

		// List all teams and their templates
		if p.JSONEnabled() {
			result := make(map[string]interface{})
			for teamKey, teamData := range metadata.Templates {
				templateNames := make([]string, 0, len(teamData.Templates))
				for _, template := range teamData.Templates {
					templateNames = append(templateNames, template.Name)
				}
				result[teamKey] = map[string]interface{}{
					"templates": templateNames,
					"last_sync": teamData.LastSync,
				}
			}
			return p.PrintJSON(result)
		}

		if len(metadata.Templates) == 0 {
			fmt.Println("No templates cached. Run 'linear-cli templates sync --all' to get started.")
			return nil
		}

		fmt.Printf("Cached templates (last sync: %v ago):\n\n", time.Since(metadata.LastSync).Round(time.Minute))
		for teamKey, teamData := range metadata.Templates {
			fmt.Printf("%s (%d templates, synced %v ago):\n", teamKey, len(teamData.Templates), time.Since(teamData.LastSync).Round(time.Minute))
			for _, template := range teamData.Templates {
				fmt.Printf("  - %s\n", template.Name)
			}
			fmt.Println()
		}

		return nil
	},
}

var templatesCleanCmd = &cobra.Command{
	Use:   "clean [--team <key>] [--all]",
	Short: "Clean up local template cache",
	Long: `Clean up local template cache files.

This removes all locally cached template files and metadata. Templates will need
to be re-synced after cleaning.

Examples:
  linear-cli templates clean --team POK    # Clean templates for team POK only
  linear-cli templates clean --all         # Clean all cached templates`,
	RunE: func(cmd *cobra.Command, args []string) error {
		teamKey, _ := cmd.Flags().GetString("team")
		cleanAll, _ := cmd.Flags().GetBool("all")

		if !cleanAll && strings.TrimSpace(teamKey) == "" {
			return errors.New("either --team <key> or --all is required")
		}

		templatesDir, err := getTemplatesDir()
		if err != nil {
			return fmt.Errorf("failed to access templates directory: %w", err)
		}

		if cleanAll {
			// Remove entire templates directory
			err := os.RemoveAll(templatesDir)
			if err != nil {
				return fmt.Errorf("failed to clean templates directory: %w", err)
			}
			fmt.Println("All template cache cleaned successfully!")
			return nil
		}

		// Clean specific team
		teamKey = strings.ToUpper(strings.TrimSpace(teamKey))
		teamDir := filepath.Join(templatesDir, teamKey)
		
		err = os.RemoveAll(teamDir)
		if err != nil {
			return fmt.Errorf("failed to clean templates for team %s: %w", teamKey, err)
		}

		// Update metadata to remove this team
		metadata, err := loadTemplateMetadata(templatesDir)
		if err == nil {
			delete(metadata.Templates, teamKey)
			_ = saveTemplateMetadata(templatesDir, metadata) // Best effort
		}

		fmt.Printf("Template cache for team %s cleaned successfully!\n", teamKey)
		return nil
	},
}

var templatesStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show template sync status",
	Long:  `Show the status of template synchronization for all teams.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		templatesDir, err := getTemplatesDir()
		if err != nil {
			return fmt.Errorf("failed to access templates directory: %w", err)
		}

		metadata, err := loadTemplateMetadata(templatesDir)
		if err != nil {
			fmt.Println("No templates synced yet. Run 'linear-cli templates sync --all' to get started.")
			return nil
		}

		p := printer(cmd)
		if p.JSONEnabled() {
			return p.PrintJSON(metadata)
		}

		if len(metadata.Templates) == 0 {
			fmt.Println("No templates synced yet. Run 'linear-cli templates sync --all' to get started.")
			return nil
		}

		fmt.Printf("Template Sync Status (last global sync: %v ago)\n\n", time.Since(metadata.LastSync).Round(time.Minute))
		
		for teamKey, teamData := range metadata.Templates {
			status := "✓ Current"
			if time.Since(teamData.LastSync) > 24*time.Hour {
				status = "⚠ Stale (>24h)"
			} else if time.Since(teamData.LastSync) > 1*time.Hour {
				status = "△ Old (>1h)"
			}

			fmt.Printf("%s: %s (%d templates, synced %v ago)\n", 
				teamKey, status, len(teamData.Templates), time.Since(teamData.LastSync).Round(time.Minute))
		}

		fmt.Println("\nRun 'linear-cli templates sync --all' to update all teams")
		fmt.Println("Run 'linear-cli templates sync --team <key>' to update a specific team")

		return nil
	},
}

// Helper functions

func getTemplatesDir() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	
	templatesDir := filepath.Join(configDir, "templates")
	err = os.MkdirAll(templatesDir, 0755)
	if err != nil {
		return "", err
	}
	
	return templatesDir, nil
}

func loadTemplateMetadata(templatesDir string) (*TemplateMetadata, error) {
	metadataPath := filepath.Join(templatesDir, ".metadata.json")
	
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}
	
	var metadata TemplateMetadata
	err = json.Unmarshal(data, &metadata)
	if err != nil {
		return nil, err
	}
	
	return &metadata, nil
}

func saveTemplateMetadata(templatesDir string, metadata *TemplateMetadata) error {
	metadataPath := filepath.Join(templatesDir, ".metadata.json")
	
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(metadataPath, data, 0644)
}

func syncTeamTemplatesIntelligent(client *api.Client, team api.Team, templatesDir string, metadata *TemplateMetadata) (*SyncResult, error) {
	// Get templates for this team
	templates, err := client.ListIssueTemplatesForTeam(team.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}

	if len(templates) == 0 {
		return &SyncResult{
			SkipReason: "No templates found",
		}, nil
	}

	// Get existing team data
	existingTeamData, hasExistingData := metadata.Templates[team.Key]
	
	// Determine what needs to be synced
	var newTemplates []api.IssueTemplate
	var updatedTemplates []api.IssueTemplate
	var removedTemplateNames []string
	
	// Check for new and updated templates
	for _, template := range templates {
		if !hasExistingData {
			newTemplates = append(newTemplates, template)
		} else if existingTemplate, exists := existingTeamData.Templates[template.Name]; !exists {
			newTemplates = append(newTemplates, template)
		} else {
			// Check if template needs updating (template ID changed or file missing)
			templatePath := filepath.Join(templatesDir, team.Key, existingTemplate.Filename)
			if existingTemplate.ID != template.ID || !fileExists(templatePath) {
				updatedTemplates = append(updatedTemplates, template)
			}
		}
	}
	
	// Check for removed templates
	if hasExistingData {
		currentTemplateNames := make(map[string]bool)
		for _, template := range templates {
			currentTemplateNames[template.Name] = true
		}
		
		for templateName := range existingTeamData.Templates {
			if !currentTemplateNames[templateName] {
				removedTemplateNames = append(removedTemplateNames, templateName)
			}
		}
	}
	
	// Skip if nothing to sync
	if len(newTemplates) == 0 && len(updatedTemplates) == 0 && len(removedTemplateNames) == 0 {
		timeSinceSync := "never"
		if hasExistingData {
			timeSinceSync = time.Since(existingTeamData.LastSync).Round(time.Minute).String() + " ago"
		}
		return &SyncResult{
			SkipReason: fmt.Sprintf("Up to date (%d templates, last synced %s)", len(templates), timeSinceSync),
		}, nil
	}

	// Perform the sync
	fmt.Printf("  Syncing %d new, %d updated, removing %d templates...\n", 
		len(newTemplates), len(updatedTemplates), len(removedTemplateNames))

	// Create team directory
	teamDir := filepath.Join(templatesDir, team.Key)
	err = os.MkdirAll(teamDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create team directory: %w", err)
	}

	// Start with existing team data or create new
	var teamTemplates TeamTemplates
	if hasExistingData {
		teamTemplates = existingTeamData
	} else {
		teamTemplates = TeamTemplates{
			TeamID:    team.ID,
			TeamKey:   team.Key,
			Templates: make(map[string]TemplateInfo),
		}
	}
	teamTemplates.LastSync = time.Now()

	// Process new templates
	for _, template := range newTemplates {
		fmt.Printf("    Adding new template: %s\n", template.Name)
		err := syncSingleTemplate(client, team, template, teamDir, &teamTemplates)
		if err != nil {
			fmt.Printf("      Warning: Failed to sync %s: %v\n", template.Name, err)
		}
	}

	// Process updated templates
	for _, template := range updatedTemplates {
		fmt.Printf("    Updating template: %s\n", template.Name)
		err := syncSingleTemplate(client, team, template, teamDir, &teamTemplates)
		if err != nil {
			fmt.Printf("      Warning: Failed to update %s: %v\n", template.Name, err)
		}
	}

	// Remove old templates
	for _, templateName := range removedTemplateNames {
		fmt.Printf("    Removing template: %s\n", templateName)
		if existingTemplate, exists := teamTemplates.Templates[templateName]; exists {
			templatePath := filepath.Join(teamDir, existingTemplate.Filename)
			_ = os.Remove(templatePath) // Best effort
			delete(teamTemplates.Templates, templateName)
		}
	}

	// Update metadata
	metadata.Templates[team.Key] = teamTemplates
	
	// Build summary
	summary := fmt.Sprintf("Synced successfully (%d new, %d updated, %d removed)", 
		len(newTemplates), len(updatedTemplates), len(removedTemplateNames))
	
	return &SyncResult{
		SyncSummary:      summary,
		NewTemplates:     len(newTemplates),
		UpdatedTemplates: len(updatedTemplates),
		RemovedTemplates: len(removedTemplateNames),
	}, nil
}

// syncSingleTemplate syncs a single template, reusing existing reference issues when possible
func syncSingleTemplate(client *api.Client, team api.Team, template api.IssueTemplate, teamDir string, teamTemplates *TeamTemplates) error {
	var refIssue *api.Issue
	
	// Check if we can reuse an existing reference issue
	if existingTemplate, exists := teamTemplates.Templates[template.Name]; exists && existingTemplate.RefIssueID != "" {
		existingIssue, err := client.IssueByID(existingTemplate.RefIssueID)
		if err == nil && existingIssue != nil {
			refIssue = existingIssue
			fmt.Printf("      Reusing reference issue: %s\n", existingIssue.Identifier)
		}
	}
	
	if refIssue == nil {
		// Create a new reference issue
		newRefIssue, err := client.CreateIssueAdvanced(api.IssueCreateInput{
			TeamID:     team.ID,
			TemplateID: template.ID,
			Title:      fmt.Sprintf("[TEMPLATE-REF] %s", template.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to create reference issue: %w", err)
		}
		refIssue = &api.Issue{
			ID:          newRefIssue.ID,
			Identifier:  newRefIssue.Identifier,
			Title:       newRefIssue.Title,
			Description: newRefIssue.Description,
			URL:         newRefIssue.URL,
		}
		fmt.Printf("      Created reference issue: %s\n", refIssue.Identifier)
	}

	// Extract template content
	templateContent := refIssue.Description
	if templateContent == "" {
		templateContent = "# " + template.Name + "\n\n(No template content available)"
	}

	// Save to file
	filename := sanitizeFilename(template.Name) + ".md"
	templatePath := filepath.Join(teamDir, filename)
	
	err := os.WriteFile(templatePath, []byte(templateContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	// Update metadata
	teamTemplates.Templates[template.Name] = TemplateInfo{
		ID:            template.ID,
		Name:          template.Name,
		Filename:      filename,
		LastSync:      time.Now(),
		Description:   templateContent,
		RefIssueID:    refIssue.ID,
		RefIssueKey:   refIssue.Identifier,
	}
	
	return nil
}

func sanitizeFilename(name string) string {
	// Convert to lowercase and replace spaces/special chars with hyphens
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	
	// Remove other special characters
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	
	return strings.Trim(result.String(), "-")
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// cleanupOldTemplateFiles removes template files that are no longer in the API
func cleanupOldTemplateFiles(teamDir string, currentTemplates []api.IssueTemplate) error {
	// Get list of current template names
	currentNames := make(map[string]bool)
	for _, template := range currentTemplates {
		filename := sanitizeFilename(template.Name) + ".md"
		currentNames[filename] = true
	}
	
	// Read existing files in team directory
	entries, err := os.ReadDir(teamDir)
	if err != nil {
		return err
	}
	
	// Remove files that don't correspond to current templates
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		filename := entry.Name()
		if strings.HasSuffix(filename, ".md") && !currentNames[filename] {
			filePath := filepath.Join(teamDir, filename)
			err := os.Remove(filePath)
			if err != nil {
				fmt.Printf("      Warning: Failed to remove old template file %s: %v\n", filename, err)
			} else {
				fmt.Printf("      Removed outdated template file: %s\n", filename)
			}
		}
	}
	
	return nil
}

// GetLocalTemplate reads a template from local storage
func GetLocalTemplate(teamKey, templateName string) (*TemplateInfo, string, error) {
	templatesDir, err := getTemplatesDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to access templates directory: %w", err)
	}

	metadata, err := loadTemplateMetadata(templatesDir)
	if err != nil {
		return nil, "", fmt.Errorf("no templates found. Run 'linear-cli templates sync --team %s' first", teamKey)
	}

	teamKey = strings.ToUpper(strings.TrimSpace(teamKey))
	teamData, exists := metadata.Templates[teamKey]
	if !exists {
		return nil, "", fmt.Errorf("no templates found for team %s. Run 'linear-cli templates sync --team %s' first", teamKey, teamKey)
	}

	template, exists := teamData.Templates[templateName]
	if !exists {
		return nil, "", fmt.Errorf("template '%s' not found for team %s", templateName, teamKey)
	}

	// Read the template file
	templatePath := filepath.Join(templatesDir, teamKey, template.Filename)
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read template file: %w", err)
	}

	return &template, string(content), nil
}

// GetLocalTemplatesForTeam returns all locally cached templates for a team
func GetLocalTemplatesForTeam(teamKey string) ([]TemplateInfo, error) {
	templatesDir, err := getTemplatesDir()
	if err != nil {
		return nil, fmt.Errorf("failed to access templates directory: %w", err)
	}

	metadata, err := loadTemplateMetadata(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("no templates found. Run 'linear-cli templates sync --team %s' first", teamKey)
	}

	teamKey = strings.ToUpper(strings.TrimSpace(teamKey))
	teamData, exists := metadata.Templates[teamKey]
	if !exists {
		return nil, fmt.Errorf("no templates found for team %s. Run 'linear-cli templates sync --team %s' first", teamKey, teamKey)
	}

	templates := make([]TemplateInfo, 0, len(teamData.Templates))
	for _, template := range teamData.Templates {
		templates = append(templates, template)
	}

	return templates, nil
}

// ParseTemplateSections extracts section names from template content
func ParseTemplateSections(content string) []string {
	lines := strings.Split(content, "\n")
	var sections []string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Match markdown headers (### Section Name)
		if strings.HasPrefix(line, "### ") {
			section := strings.TrimPrefix(line, "### ")
			section = strings.TrimSpace(section)
			if section != "" {
				sections = append(sections, section)
			}
		}
		
		// Match lines ending with colon (Section:)
		if strings.HasSuffix(line, ":") && !strings.Contains(line, " ") {
			section := strings.TrimSuffix(line, ":")
			section = strings.TrimSpace(section)
			if section != "" && !strings.HasPrefix(section, "#") {
				sections = append(sections, section)
			}
		}
	}
	
	return sections
}

func init() {
	// Add flags
	templatesSyncCmd.Flags().String("team", "", "Team key to sync templates for")
	templatesSyncCmd.Flags().Bool("all", false, "Sync templates for all accessible teams")

	templatesListCmd.Flags().String("team", "", "Team key to list templates for")

	templatesCleanCmd.Flags().String("team", "", "Team key to clean templates for")
	templatesCleanCmd.Flags().Bool("all", false, "Clean all cached templates")

	// Add subcommands
	templatesCmd.AddCommand(templatesSyncCmd)
	templatesCmd.AddCommand(templatesListCmd)
	templatesCmd.AddCommand(templatesCleanCmd)
	templatesCmd.AddCommand(templatesStatusCmd)

	// Add to root command
	rootCmd.AddCommand(templatesCmd)
}
