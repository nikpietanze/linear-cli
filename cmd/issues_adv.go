package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

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

// Template utilities 
// list available templates and preview a template by name or path
var issuesTemplateCmd = &cobra.Command{
    Use:   "template",
    Short: "Work with issue templates",
    Long: `Manage and inspect issue templates from local directories or a remote base URL.

Commands:
  list                List available template names
  preview <name|path> Render a template with optional --var/--vars-file substitutions

Template format:
  - Optional first line: 'Title-Prefix: <prefix>' to auto-prefix issue titles
  - Placeholders: {{KEY}} or {{KEY|Prompt text...}} used with 'issues create --template'

Sources:
  - Local directories (search order): --templates-dir, $LINEAR_TEMPLATES_DIR, UserConfigDir/linear/templates, ~/.config/linear/templates
  - Remote base URL: --templates-base-url or $LINEAR_TEMPLATES_BASE_URL. Names resolve to <base>/<name>.md and list reads <base>/index.json`,
    RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

// issuesTemplateStructureCmd shows template sections for AI agents
var issuesTemplateStructureCmd = &cobra.Command{
    Use:   "structure --team <key> [--template <name>]",
    Short: "Show available templates and their structure for AI agents",
    Long: `Shows available templates for a team and their section structure.
This helps AI agents understand what templates exist and what sections are available.

Without --template: Lists all available templates for the team
With --template: Shows the section structure for a specific template

Example output:
  Available templates for team ENG:
  - Feature Template
  - Bug Template  
  - Enhancement Template
  - Research Template

  Structure for "Feature Template":
  - Summary
  - Context
  - Requirements
  - Definition of Done

AI agents can then use: --template "Feature Template" --sections Summary="Brief desc"`,
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, _ := config.Load()
        if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
        client := api.NewClient(cfg.APIKey)
        
        teamKey, _ := cmd.Flags().GetString("team")
        templateName, _ := cmd.Flags().GetString("template")
        
        if strings.TrimSpace(teamKey) == "" { return errors.New("--team is required") }
        
        // Get team
        team, err := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
        if err != nil { return err }
        if team == nil { return fmt.Errorf("team with key %s not found", teamKey) }
        
        // If no specific template requested, list all available templates
        if strings.TrimSpace(templateName) == "" {
            templates, err := client.ListIssueTemplatesForTeam(team.ID)
            if err != nil { return err }
            
            p := printer(cmd)
            if p.JSONEnabled() {
                templateNames := make([]string, len(templates))
                for i, t := range templates {
                    templateNames[i] = t.Name
                }
                return p.PrintJSON(map[string]interface{}{
                    "team": teamKey,
                    "templates": templateNames,
                })
            }
            
            fmt.Printf("Available templates for team %s:\n", teamKey)
            for _, template := range templates {
                fmt.Printf("  - %s\n", template.Name)
            }
            fmt.Printf("\nTo see structure: linear-cli issues template structure --team %s --template \"Template Name\"\n", teamKey)
            return nil
        }
        
        // Get template info from local cache (no temporary issues needed!)
        templateInfo, templateContent, err := GetLocalTemplate(teamKey, templateName)
        if err != nil {
            return fmt.Errorf("template not found locally. Run 'linear-cli templates sync --team %s' first. Error: %w", teamKey, err)
        }
        
        // Parse sections from cached template content
        sections := ParseTemplateSections(templateContent)
        
        p := printer(cmd)
        if p.JSONEnabled() {
            return p.PrintJSON(map[string]interface{}{
                "template": templateInfo.Name,
                "sections": sections,
                "example": fmt.Sprintf("--template \"%s\" --sections %s", templateInfo.Name, buildExampleSections(sections)),
            })
        }
        
        fmt.Printf("Template: %s\n", templateInfo.Name)
        fmt.Printf("Available sections:\n")
        for _, section := range sections {
            fmt.Printf("  - %s\n", section)
        }
        fmt.Printf("\nExample usage:\n")
        fmt.Printf("  linear-cli issues create --team %s --template \"%s\" --title \"Your title\" \\\n", teamKey, templateInfo.Name)
        fmt.Printf("    --sections %s\n", buildExampleSections(sections))
        
        return nil
    },
}

// issuesTemplateShowCmd prints template title and description by name from API for a given team
var issuesTemplateShowCmd = &cobra.Command{
    Use:   "show",
    Short: "Show a template's title and description from the API",
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, _ := config.Load()
        if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
        teamKey, _ := cmd.Flags().GetString("team")
        name, _ := cmd.Flags().GetString("name")
        if strings.TrimSpace(teamKey) == "" || strings.TrimSpace(name) == "" {
            return errors.New("--team and --name are required")
        }
        client := api.NewClient(cfg.APIKey)
        t, err := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
        if err != nil { return err }
        if t == nil { return fmt.Errorf("team with key %s not found", teamKey) }
        // Try dynamic full first
        title, body, err := client.IssueTemplateByNameForTeamFull(t.ID, name)
        if err != nil { return err }
        // If body is still empty (or literal "null"), dump raw node fields for debugging
        if strings.TrimSpace(body) == "" || strings.EqualFold(strings.TrimSpace(body), "null") {
            items, _ := client.ListIssueTemplatesForTeam(t.ID)
            var picked *api.IssueTemplate
            for _, it := range items { if strings.EqualFold(strings.TrimSpace(it.Name), strings.TrimSpace(name)) { picked = &it; break } }
            if picked == nil {
                for _, it := range items { if strings.Contains(strings.ToLower(strings.TrimSpace(it.Name)), strings.ToLower(strings.TrimSpace(name))) { picked = &it; break } }
            }
            if picked != nil {
                node, _ := client.TemplateNodeByIDRaw(picked.ID)
                return printer(cmd).PrintJSON(map[string]any{"title": picked.Name, "node": node})
            }
            return fmt.Errorf("template '%s' not found or has no body", name)
        }
        p := printer(cmd)
        if p.JSONEnabled() { return p.PrintJSON(map[string]any{"title": title, "description": body}) }
        fmt.Printf("%s\n\n%s\n", title, strings.TrimSpace(body))
        return nil
    },
}

var issuesTemplateListCmd = &cobra.Command{
    Use:   "list",
    Short: "List available templates",
    RunE: func(cmd *cobra.Command, args []string) error {
        override, _ := cmd.Flags().GetString("templates-dir")
        baseOverride, _ := cmd.Flags().GetString("templates-base-url")
        source, _ := cmd.Flags().GetString("templates-source")
        teamKey, _ := cmd.Flags().GetString("team")
        
        // If team is provided and source is auto, prefer API
        if source == "auto" && strings.TrimSpace(teamKey) != "" {
            cfg, _ := config.Load()
            if cfg.APIKey != "" {
                source = "api"
            }
        } else if source == "api" || (source == "auto") {
            // Auto-prefer API when available
            cfg, _ := config.Load()
            if cfg.APIKey != "" {
                client := api.NewClient(cfg.APIKey)
                if client.SupportsIssueTemplates() { source = "api" }
            }
        }
        if source == "api" {
            cfg, _ := config.Load()
            if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
            if strings.TrimSpace(teamKey) == "" { return errors.New("--team is required with --templates-source=api") }
            client := api.NewClient(cfg.APIKey)
            // Resolve team key to ID
            t, err := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
            if err != nil { return err }
            if t == nil { return fmt.Errorf("team with key %s not found", teamKey) }
            items, err := client.ListIssueTemplatesForTeam(t.ID)
            if err != nil { return err }
            names := make([]string, 0, len(items))
            for _, it := range items { names = append(names, it.Name) }
            p := printer(cmd)
            if p.JSONEnabled() { return p.PrintJSON(map[string]any{"templates": names}) }
            for _, n := range names { fmt.Println(n) }
            if len(names) == 0 { fmt.Println("No templates found for team", teamKey) }
            return nil
        }
        dirs := templateSearchDirs(override)
        seen := map[string]struct{}{}
        names := []string{}
        for _, dir := range dirs {
            entries, err := os.ReadDir(dir)
            if err != nil { continue }
            for _, e := range entries {
                if e.IsDir() { continue }
                name := e.Name()
                if strings.HasSuffix(strings.ToLower(name), ".md") {
                    base := strings.TrimSuffix(name, ".md")
                    if _, ok := seen[base]; !ok {
                        seen[base] = struct{}{}
                        names = append(names, base)
                    }
                }
            }
        }

        // Remote index support: <base>/index.json either as ["bug","feature"] or {"templates":[...]} 
        if base := templateBaseURL(baseOverride); base != "" {
            idxURL := joinURL(base, "index.json")
            if content, err := fetchURL(idxURL); err == nil {
                var arr []string
                var obj struct{ Templates []string `json:"templates"` }
                if json.Unmarshal([]byte(content), &arr) == nil {
                    for _, n := range arr { n = strings.TrimSpace(n); if n == "" { continue }; if _, ok := seen[n]; !ok { seen[n] = struct{}{}; names = append(names, n) } }
                } else if json.Unmarshal([]byte(content), &obj) == nil {
                    for _, n := range obj.Templates { n = strings.TrimSpace(n); if n == "" { continue }; if _, ok := seen[n]; !ok { seen[n] = struct{}{}; names = append(names, n) } }
                }
            }
        }
        p := printer(cmd)
        if p.JSONEnabled() { return p.PrintJSON(map[string]any{"templates": names}) }
        if len(names) == 0 {
            fmt.Println("No templates found. Searched:")
            for _, d := range dirs { fmt.Println(" -", d) }
            if base := templateBaseURL(baseOverride); base != "" { fmt.Println(" - Remote base:", base) }
            return nil
        }
        for _, n := range names { fmt.Println(n) }
        return nil
    },
}

var issuesTemplatePreviewCmd = &cobra.Command{
    Use:   "preview <name-or-path>",
    Short: "Preview a template after optional variable substitution",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        override, _ := cmd.Flags().GetString("templates-dir")
        baseOverride, _ := cmd.Flags().GetString("templates-base-url")
        source, _ := cmd.Flags().GetString("templates-source")
        teamKey, _ := cmd.Flags().GetString("team")
        debug, _ := cmd.Flags().GetBool("debug")
        
        // If team is provided and source is auto, prefer API
        if source == "auto" && strings.TrimSpace(teamKey) != "" {
            cfg, _ := config.Load()
            if cfg.APIKey != "" {
                source = "api"
            }
        } else if source == "api" || (source == "auto") {
            cfg, _ := config.Load()
            if cfg.APIKey != "" {
                client := api.NewClient(cfg.APIKey)
                if client.SupportsIssueTemplates() { source = "api" }
            }
        }
        var raw string
        var tplTitle string
        var err error
        if source == "api" {
            cfg, _ := config.Load()
            if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
            client := api.NewClient(cfg.APIKey)
            // Resolve team id
            if strings.TrimSpace(teamKey) == "" { return errors.New("--team is required to resolve template by name with --templates-source=api") }
            t, errT := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
            if errT != nil { return errT }
            if t == nil { return fmt.Errorf("team with key %s not found", teamKey) }

            // Prefer listing team templates and matching by name (works across schema variants)
            if items, e := client.ListIssueTemplatesForTeam(t.ID); e == nil && len(items) > 0 {
                // Robust normalize: lowercase, remove spaces and punctuation
                normalize := func(s string) string {
                    s = strings.ToLower(strings.TrimSpace(s))
                    var b strings.Builder
                    for _, r := range s {
                        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') { b.WriteRune(r); continue }
                    }
                    return b.String()
                }
                name := strings.TrimSpace(args[0])
                normName := normalize(name)
                // Exact normalized match first, then contains
                for _, it := range items {
                    if normalize(it.Name) == normName {
                        tplTitle = it.Name
                        raw = it.Description
                        break
                    }
                }
                if strings.TrimSpace(raw) == "" {
                    for _, it := range items {
                        if strings.Contains(normalize(it.Name), normName) {
                            tplTitle = it.Name
                            raw = it.Description
                            break
                        }
                    }
                }
                // If still empty, fetch full template by ID to retrieve description
                if strings.TrimSpace(raw) == "" {
                    for _, it := range items {
                        if (tplTitle != "" && it.Name == tplTitle) || normalize(it.Name) == normName {
                            if got, e := client.IssueTemplateByID(it.ID); e == nil && got != nil {
                                if tplTitle == "" { tplTitle = got.Name }
                                raw = got.Description
                            }
                            break
                        }
                    }
                }
                if debug {
                    cand := make([]string, 0, len(items))
                    for _, it := range items { cand = append(cand, it.Name) }
                    _ = printer(cmd).PrintJSON(map[string]any{"debug": true, "teamId": t.ID, "candidates": cand, "requested": name})
                }
            }
            // Fallback: try by id or name via direct resolvers if list path failed
            if strings.TrimSpace(raw) == "" {
                if tpl, e := client.IssueTemplateByID(args[0]); e == nil && tpl != nil {
                    tplTitle = tpl.Name
                    raw = tpl.Description
                } else {
                    tpl, errN := client.IssueTemplateByNameForTeam(t.ID, args[0])
                    if errN == nil && tpl != nil {
                        tplTitle = tpl.Name
                        raw = tpl.Description
                    }
                }
            }
            if strings.TrimSpace(raw) == "" {
                return fmt.Errorf("template '%s' not found for team %s", args[0], teamKey)
            }
        } else {
            raw, err = loadTemplateContent(args[0], override, baseOverride)
            if err != nil { return err }
        }
        varsKVs, _ := cmd.Flags().GetStringArray("var")
        varsFile, _ := cmd.Flags().GetString("vars-file")
        vars, err := gatherVars(varsKVs, varsFile)
        if err != nil { return err }
        // Non-interactive preview; do not fail on missing by default
        rendered, err := fillTemplate(raw, vars, false, false)
        if err != nil { return err }
        p := printer(cmd)
        if p.JSONEnabled() {
            // When using API source, include template title for clarity
            if tplTitle != "" { return p.PrintJSON(map[string]any{"title": tplTitle, "description": rendered}) }
            return p.PrintJSON(map[string]any{"description": rendered})
        }
        if tplTitle != "" { fmt.Printf("%s\n\n", tplTitle) }
        fmt.Println(rendered)
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
    Long:  "List issues with optional filters for project, assignee, and state. Use convenience shortcuts --todo/--doing/--done or explicit --state.",
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
    Use:   "create --team <key> [flags]",
    Short: "Create a new issue",
    Long: `🤖 AI-OPTIMIZED ISSUE CREATION

Create fully structured Linear issues in a single command. Designed for AI agents, automation workflows, and programmatic issue management.

✨ FEATURES:
  • Single-command issue creation with template auto-discovery
  • Automatic template synchronization and intelligent caching  
  • Dynamic section filling that adapts to any team's templates
  • Server-side template application for Linear consistency
  • JSON output for programmatic integration

🚀 AI AGENT WORKFLOW:
  1. Create structured issues instantly (auto-discovery included):
     linear-cli issues create --team TEAM --template "Template Name" --title "Title" \
       --sections Summary="Brief description" Context="Background info"
  
  2. Discover templates (optional):
     linear-cli issues template structure --team TEAM
  
  Example:
    linear-cli issues template structure --team ENG
    linear-cli issues template structure --team ENG --template "Feature Template"
    linear-cli issues create --team ENG --template "Feature Template" --title "Add dark mode" \
      --sections Summary="Add dark theme toggle" Context="Users need low-light option"

👤 INTERACTIVE WORKFLOW:
  1. Run: linear-cli issues create --team TEAM
  2. Select issue type (Feature/Bug/Spike) 
  3. Enter title (auto-prefixed: "Feat:", "Bug:", "Spike:")
  4. Fill template sections interactively

🔧 TECHNICAL DETAILS:
  - Templates applied server-side by Linear's API (ensures consistency)
  - Auto-detects issue type from title keywords if --type not specified
  - Default priority: Medium (3), Default state: Todo/Backlog
  - Supports any team's template structure dynamically

🚀 SETUP FOR AI AGENTS:
  1. Authenticate: linear-cli auth login
  2. Test connection: linear-cli auth status  
  3. Discover team templates: linear-cli issues template structure --team TEAM
  4. Get template structure: linear-cli issues template structure --team TEAM --template "Name"
  5. Create structured issues: Use --template and --sections flags

The CLI automatically handles template application, labeling, and state management to ensure 
issues are created exactly as if done through Linear's web interface.`,
    Example: `  # AI Agent: Discover available templates
  linear-cli issues template structure --team ENG
  
  # AI Agent: Get specific template structure  
  linear-cli issues template structure --team ENG --template "Feature Template"
  
  # AI Agent: Create structured issue
  linear-cli issues create --team ENG --template "Feature Template" --title "Add dark mode" \
    --sections Summary="Add dark theme toggle" Context="Users need low-light option"
  
  # Interactive creation
  linear-cli issues create --team ENG`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
		client := api.NewClient(cfg.APIKey)

        title, _ := cmd.Flags().GetString("title")
        description, _ := cmd.Flags().GetString("description")
        templateName, _ := cmd.Flags().GetString("template")
        templateID, _ := cmd.Flags().GetString("template-id")
        interactiveFlag, _ := cmd.Flags().GetBool("interactive")
        noInteractive, _ := cmd.Flags().GetBool("no-interactive")
        
        // AI-friendly template section flags  
        sections, _ := cmd.Flags().GetStringToString("sections")
        previewFlag, _ := cmd.Flags().GetBool("preview")
        noPreview, _ := cmd.Flags().GetBool("no-preview")
        yes, _ := cmd.Flags().GetBool("yes")
        failOnMissing, _ := cmd.Flags().GetBool("fail-on-missing")
        varsKVs, _ := cmd.Flags().GetStringArray("var")
        varsFile, _ := cmd.Flags().GetString("vars-file")
		project, _ := cmd.Flags().GetString("project")
        teamKey, _ := cmd.Flags().GetString("team")
        templatesDir, _ := cmd.Flags().GetString("templates-dir")
        baseOverride, _ := cmd.Flags().GetString("templates-base-url")
        source, _ := cmd.Flags().GetString("templates-source")
		assignee, _ := cmd.Flags().GetString("assignee")
		label, _ := cmd.Flags().GetString("label")
		priority, _ := cmd.Flags().GetInt("priority")
        // Title can be gathered interactively if not provided
        // Compute default behavior: interactive by default with templates unless explicitly disabled.
        // If prefill vars are provided, default to preview unless explicitly disabled.
        varsProvided := len(varsKVs) > 0 || strings.TrimSpace(varsFile) != ""
        // Determine if this is AI-friendly mode
        isAIMode := strings.TrimSpace(templateName) != "" && len(sections) > 0 && strings.TrimSpace(title) != ""
        
        // Interactive is the default unless explicitly disabled or in AI mode
        interactive := interactiveFlag
        if !noInteractive && !cmd.Flags().Changed("interactive") && !isAIMode {
            interactive = true
        } else if isAIMode {
            interactive = false
        }
        preview := previewFlag
        if varsProvided && !noPreview && !cmd.Flags().Changed("preview") {
            preview = true
        }

        // AI-FRIENDLY MODE: Seamless single-command creation
        if isAIMode {
            if strings.TrimSpace(teamKey) == "" {
                return errors.New("--team is required for AI-friendly mode")
            }
            
            return createIssueAIFriendly(client, teamKey, templateName, title, sections, cmd)
        }

        // If user requested interactive but provided no template or description, offer to pick a template
        if interactive && strings.TrimSpace(templateName) == "" && strings.TrimSpace(description) == "" {
            tmpl, pickErr := interactivePickTemplate(cmd, client, teamKey)
            if pickErr == nil && strings.TrimSpace(tmpl) != "" { templateName = tmpl }
        }

        // Fast path: server-side creation from API template id
        if strings.TrimSpace(templateID) != "" {
            if strings.TrimSpace(teamKey) == "" { return errors.New("--team is required when using --template-id") }
            t, errT := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
            if errT != nil { return errT }
            if t == nil { return fmt.Errorf("team with key %s not found", teamKey) }
            created, err := client.CreateIssueFromTemplate(t.ID, templateID, title)
            if err != nil { return err }
            p := printer(cmd)
            if p.JSONEnabled() { return p.PrintJSON(created) }
            fmt.Printf("Created %s: %s\n", created.Identifier, created.URL)
            return nil
        }

        // Load template and optionally fill it
        // For interactive runs, defer template loading until after type selection so we can auto-pick by kind
        if !interactive && strings.TrimSpace(description) == "" && strings.TrimSpace(templateName) != "" {
            var tplContent string
            var err error
            if source == "api" {
                // Fetch template content via API, resolving by id or by name within team
                client := api.NewClient(cfg.APIKey)
                if tpl, e := client.IssueTemplateByID(templateName); e == nil && tpl != nil {
                    tplContent = tpl.Description
                } else {
                    if strings.TrimSpace(teamKey) == "" { return errors.New("--team is required to resolve template by name with --templates-source=api") }
                    t, errT := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
                    if errT != nil { return errT }
                    if t == nil { return fmt.Errorf("team with key %s not found", teamKey) }
                    tpl, errN := client.IssueTemplateByNameForTeam(t.ID, templateName)
                    if errN != nil { return errN }
                    if tpl == nil { return fmt.Errorf("template '%s' not found for team %s", templateName, teamKey) }
                    tplContent = tpl.Description
                }
            } else {
                tplContent, err = loadTemplateContent(templateName, templatesDir, baseOverride)
                if err != nil { return fmt.Errorf("failed to load template '%s': %w", templateName, err) }
            }
            // Extract optional title prefix metadata and strip it from the template body
            if prefix, body := parseTitlePrefixAndStrip(tplContent); prefix != "" {
                if !strings.HasPrefix(strings.TrimSpace(title), prefix) {
                    title = strings.TrimSpace(prefix + " " + title)
                }
                tplContent = body
            }
            vars, err := gatherVars(varsKVs, varsFile)
            if err != nil { return err }
            description, err = fillTemplate(tplContent, vars, interactive, failOnMissing)
            if err != nil { return err }
            // If template had no placeholders and description is still empty, prompt by sections
            if interactive && strings.TrimSpace(description) == "" && !hasTemplatePlaceholders(tplContent) {
                description = promptSectionsFromTemplate(tplContent)
            }
        }

        // (moved) Description prompting happens later within the interactive walkthrough,
        // after type/template/title so it aligns with the intended flow.
        var projectID string
        var teamID string
        if project != "" {
            pr, err := client.ResolveProject(project)
            if err != nil { return err }
            if pr == nil { return fmt.Errorf("project '%s' not found", project) }
            projectID = pr.ID
            if pr.TeamID != "" { teamID = pr.TeamID }
        }
        if teamKey != "" && teamID == "" {
            t, err := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
            if err != nil { return err }
            if t == nil { return fmt.Errorf("team with key %s not found", teamKey) }
            teamID = t.ID
        }
        if teamID == "" { return errors.New("--team is required") }
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
        // Load last-used preferences for this team as defaults where applicable
        teamKeyNorm := strings.ToUpper(strings.TrimSpace(teamKey))
        tp := cfg.TeamPrefs[teamKeyNorm]
        // Do not auto-apply Urgent (1) by default; prefer 2 unless explicitly set this run
        if prioPtr == nil && tp.LastPriority >= 2 { v := tp.LastPriority; prioPtr = &v }
        if preview && strings.TrimSpace(description) != "" {
            // Show the rendered description and exit
            p := printer(cmd)
            if p.JSONEnabled() {
                _ = p.PrintJSON(map[string]any{"title": title, "projectId": projectID, "teamId": teamID, "assigneeId": assigneeID, "labelIds": labelIDs, "priority": prioPtr, "description": description})
            } else {
                fmt.Println("--- Issue Preview ---")
                fmt.Printf("Title: %s\n", title)
                if projectID != "" { fmt.Printf("ProjectID: %s\n", projectID) }
                if teamID != "" { fmt.Printf("TeamID: %s\n", teamID) }
                if assigneeID != "" { fmt.Printf("AssigneeID: %s\n", assigneeID) }
                if len(labelIDs) > 0 { fmt.Printf("Labels: %s\n", strings.Join(labelIDs, ",")) }
                if prioPtr != nil { fmt.Printf("Priority: %d\n", *prioPtr) }
                fmt.Println()
                fmt.Println(description)
            }
            if !yes {
                return nil
            }
        }
        // Track issue type for template selection
        var kind string
        
        // If interactive and still missing, walk through all fields in this order to match the desired UX
        if interactive {
            // Kind first, so we can apply prefixes and pick templates by type
            kind = promptChoiceStrict("Issue type", []string{"Feature", "Bug", "Spike"}, false)
            // Title next (so we can apply any template/type prefix consistently)
            if strings.TrimSpace(title) == "" { title = promptLine("Title: ") }
            if strings.TrimSpace(kind) != "" {
                var pref string
                switch strings.ToLower(kind) {
                case "feature": pref = "Feat:"
                case "bug": pref = "Bug:"
                case "spike": pref = "Spike:"
                }
                if pref != "" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), strings.ToLower(pref)) {
                    title = strings.TrimSpace(pref + " " + title)
                }
            }
            
            // Interactive section filling for template-based issues
            if client.SupportsIssueCreateTemplateId() && strings.TrimSpace(kind) != "" {
                // Find the template for this issue type
                if tpl, _ := client.FindTemplateForTeamByKeywords(teamID, []string{kind, kind + " template"}); tpl != nil {
                    // Create issue with server-side template first to get the structure
                    var chosenStateID string
                    if states, _ := client.TeamStates(teamID); len(states) > 0 {
                        idByName := map[string]string{}
                        for _, s := range states { idByName[s.Name] = s.ID }
                        if id, ok := idByName["Todo"]; ok { chosenStateID = id } else if id, ok := idByName["Backlog"]; ok { chosenStateID = id } else { chosenStateID = states[0].ID }
                    }
                    
                    // Set default priority to Medium (3)
                    if prioPtr == nil { v := 3; prioPtr = &v }
                    
                    // Create with template to get structure
                    tempIssue, err := client.CreateIssueAdvanced(api.IssueCreateInput{
                        ProjectID: projectID, 
                        TeamID: teamID, 
                        StateID: chosenStateID, 
                        TemplateID: tpl.ID, 
                        Title: title, 
                        AssigneeID: assigneeID, 
                        LabelIDs: labelIDs, 
                        Priority: prioPtr,
                    })
                    if err != nil { return err }
                    
                    // If user provided description, intelligently fill template sections
                    var filledDescription string
                    if strings.TrimSpace(description) != "" {
                        filledDescription = fillTemplateFromDescription(tempIssue.Description, description)
                    } else {
                        // Interactive prompting for each section
                        filledDescription = promptTemplateInteractively(tempIssue.Description)
                    }
                    
                    // Update the issue with filled content
                    if filledDescription != tempIssue.Description {
                        updatedIssue, err := client.UpdateIssue(tempIssue.ID, "", filledDescription)
                        if err != nil { return err }
                        tempIssue = updatedIssue
                    }
                    
                    p := printer(cmd)
                    if p.JSONEnabled() { return p.PrintJSON(tempIssue) }
                    fmt.Printf("Created %s: %s\n", tempIssue.Identifier, tempIssue.URL)
                    return nil
                }
            }
            
            // Set default priority to Medium (3) without prompting
            if prioPtr == nil { v := 3; prioPtr = &v }

            // Optional editor for final tweaks when a description exists
            if strings.TrimSpace(description) != "" {
                if promptYesNo("Open in editor to finalize description? (y/N): ", false) {
                    if edited, err := openInEditor(description); err == nil { description = edited }
                }
            }
        }

        // Persist last selections per team (best effort)
        if teamKey != "" {
            if cfg.TeamPrefs == nil { cfg.TeamPrefs = map[string]config.TeamPrefs{} }
            tp := cfg.TeamPrefs[strings.ToUpper(strings.TrimSpace(teamKey))]
            if projectID != "" { tp.LastProjectID = projectID }
            if assigneeID != "" { tp.LastAssigneeID = assigneeID }
            if prioPtr != nil { tp.LastPriority = *prioPtr }
            if len(labelIDs) > 0 { tp.LastLabels = labelIDs }
            if strings.TrimSpace(templateName) != "" { tp.LastTemplate = templateName }
            cfg.TeamPrefs[strings.ToUpper(strings.TrimSpace(teamKey))] = tp
            _ = config.Save(cfg)
        }

        // Final: create with server-side template application and silent state defaults
        var chosenStateID string
        if states, _ := client.TeamStates(teamID); len(states) > 0 {
            idByName := map[string]string{}
            for _, s := range states { idByName[s.Name] = s.ID }
            if id, ok := idByName["Todo"]; ok { chosenStateID = id } else if id, ok := idByName["Backlog"]; ok { chosenStateID = id } else { chosenStateID = states[0].ID }
        }
        
        // AI-friendly mode: use --template and --sections to create structured issues
        if !interactive && strings.TrimSpace(templateName) != "" && len(sections) > 0 {
            // Find template by name
            if tpl, _ := client.IssueTemplateByNameForTeam(teamID, templateName); tpl != nil {
                // Create issue with template to get structure
                tempIssue, err := client.CreateIssueAdvanced(api.IssueCreateInput{
                    ProjectID: projectID, 
                    TeamID: teamID, 
                    StateID: chosenStateID, 
                    TemplateID: tpl.ID, 
                    Title: title, 
                    AssigneeID: assigneeID, 
                    LabelIDs: labelIDs, 
                    Priority: prioPtr,
                })
                if err != nil { return err }
                
                // Fill template sections dynamically
                filledDescription := fillTemplateSectionsDynamically(tempIssue.Description, sections)
                
                // Update the issue with filled content
                if filledDescription != tempIssue.Description {
                    updatedIssue, err := client.UpdateIssue(tempIssue.ID, "", filledDescription)
                    if err != nil { return err }
                    tempIssue = updatedIssue
                }
                
                p := printer(cmd)
                if p.JSONEnabled() { return p.PrintJSON(tempIssue) }
                fmt.Printf("Created %s: %s\n", tempIssue.Identifier, tempIssue.URL)
                return nil
            }
        }
        
        // For non-interactive flows, use server-side template application if no description provided
        var templateIDForServer string
        if !interactive && client.SupportsIssueCreateTemplateId() && strings.TrimSpace(description) == "" && strings.TrimSpace(templateName) != "" {
            // Use specified template name
            if tpl, _ := client.IssueTemplateByNameForTeam(teamID, templateName); tpl != nil {
                templateIDForServer = tpl.ID
            }
        }
        
        created, err := client.CreateIssueAdvanced(api.IssueCreateInput{ProjectID: projectID, TeamID: teamID, StateID: chosenStateID, TemplateID: templateIDForServer, Title: title, Description: description, AssigneeID: assigneeID, LabelIDs: labelIDs, Priority: prioPtr})
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
    issuesCmd.AddCommand(issuesTemplateCmd)
    issuesTemplateCmd.AddCommand(issuesTemplateStructureCmd)

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

    issuesCreateAdvCmd.Flags().String("title", "", "Issue title (prompted if not provided)")
    issuesCreateAdvCmd.Flags().String("description", "", "Issue description")
    issuesCreateAdvCmd.Flags().String("template", "", "Template name (e.g. bug, feature, spike) or file path")
    issuesCreateAdvCmd.Flags().String("template-id", "", "Linear API template id to use for server-side creation (requires --team)")
    issuesCreateAdvCmd.Flags().BoolP("interactive", "i", false, "Interactive walkthrough (default: on; disable with --no-interactive)")
    issuesCreateAdvCmd.Flags().Bool("no-interactive", false, "Disable interactive walkthrough")
    
    // AI-friendly template section flags
    issuesCreateAdvCmd.Flags().StringToString("sections", nil, "Template sections as key=value pairs (e.g. --sections Summary='Brief description' Context='Background info')")
    issuesCreateAdvCmd.Flags().Bool("preview", false, "Preview the rendered issue and exit without creating (default: on when --var/--vars-file provided)")
    issuesCreateAdvCmd.Flags().Bool("no-preview", false, "Disable automatic preview when vars are provided")
    issuesCreateAdvCmd.Flags().BoolP("yes", "y", false, "Proceed with creation after preview without prompting")
    issuesCreateAdvCmd.Flags().Bool("fail-on-missing", false, "Fail if any template placeholders remain unresolved")
    issuesCreateAdvCmd.Flags().StringArray("var", nil, "Template variable assignment key=value (repeatable)")
    issuesCreateAdvCmd.Flags().String("vars-file", "", "JSON file with string key-value pairs for template variables")
    issuesCreateAdvCmd.Flags().String("project", "", "Project name or id")
    issuesCreateAdvCmd.Flags().String("team", "", "Team key (e.g. ENG)")
    issuesCreateAdvCmd.Flags().String("assignee", "", "Assignee name or id")
    issuesCreateAdvCmd.Flags().String("label", "", "Label name")
    issuesCreateAdvCmd.Flags().Int("priority", 0, "Priority (1 highest .. 4 lowest)")
    issuesCreateAdvCmd.Flags().String("templates-dir", "", "Override templates directory (default search: $LINEAR_TEMPLATES_DIR, UserConfigDir/linear/templates, ~/.config/linear/templates)")
    issuesCreateAdvCmd.Flags().String("templates-base-url", "", "Remote templates base URL (fallback: $LINEAR_TEMPLATES_BASE_URL). Names resolve to <base>/<name>.md")
    issuesCreateAdvCmd.Flags().String("templates-source", "auto", "Template source: auto|local|remote|api")
    issuesViewCmd.Flags().Int("comments", 0, "Include up to N comments")
    issuesTemplateStructureCmd.Flags().String("team", "", "Team key (required)")
    issuesTemplateStructureCmd.Flags().String("template", "", "Template name (optional - if not provided, lists all templates)")
}

// loadTemplateContent resolves a template by name, path, or URL.
// - If value is an http(s) URL, it is fetched directly
// - If value looks like a path, it is read from disk
// - Otherwise, it is treated as a name and resolved from local dirs or a remote base URL
func loadTemplateContent(value string, overrideDir string, baseOverride string) (string, error) {
    v := strings.TrimSpace(value)
    if v == "" { return "", nil }
    if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
        return fetchURL(v)
    }
    // Expand ~
    expandHome := func(p string) string {
        if strings.HasPrefix(p, "~") {
            if home, err := os.UserHomeDir(); err == nil {
                return filepath.Join(home, strings.TrimPrefix(p, "~"))
            }
        }
        return p
    }
    // If it's a path-like string, read it directly
    pathLike := strings.Contains(v, string(os.PathSeparator)) || strings.HasPrefix(v, ".") || strings.HasPrefix(v, "~")
    if pathLike {
        p := expandHome(v)
        b, err := os.ReadFile(p)
        if err != nil { return "", err }
        return string(b), nil
    }
    // Try remote base first if provided
    if base := templateBaseURL(baseOverride); base != "" {
        url := joinURL(base, v+".md")
        if s, err := fetchURL(url); err == nil { return s, nil }
    }
    // Resolve from local directories
    dirs := templateSearchDirs(overrideDir)
    for _, dir := range dirs {
        cand := filepath.Join(dir, v+".md")
        if b, err := os.ReadFile(cand); err == nil {
            return string(b), nil
        }
    }
    // If remote base exists, mention it in error for clarity
    base := templateBaseURL(baseOverride)
    if base != "" {
        return "", fmt.Errorf("template '%s' not found. Searched local: %s and remote: %s", v, strings.Join(dirs, ", "), joinURL(base, v+".md"))
    }
    return "", fmt.Errorf("template '%s' not found in any of: %s", v, strings.Join(dirs, ", "))
}

// templateSearchDirs returns candidate directories to look for templates in priority order.
func templateSearchDirs(override string) []string {
    dirs := []string{}
    if strings.TrimSpace(override) != "" {
        dirs = append(dirs, expandUserPath(override))
    }
    if env := strings.TrimSpace(os.Getenv("LINEAR_TEMPLATES_DIR")); env != "" {
        dirs = append(dirs, expandUserPath(env))
    }
    if cfg, err := os.UserConfigDir(); err == nil {
        dirs = append(dirs, filepath.Join(cfg, "linear", "templates"))
        // XDG-like fallback: ~/.config/linear/templates
        if home, err := os.UserHomeDir(); err == nil {
            dirs = append(dirs, filepath.Join(home, ".config", "linear", "templates"))
        }
    }
    return dirs
}

func expandUserPath(p string) string {
    if strings.HasPrefix(p, "~") {
        if home, err := os.UserHomeDir(); err == nil {
            return filepath.Join(home, strings.TrimPrefix(p, "~"))
        }
    }
    return p
}

// interactivePickTemplate offers the user a list of available templates (from auto/remote/local/API based on flags/env) and returns the chosen name.
func interactivePickTemplate(cmd *cobra.Command, client *api.Client, teamKey string) (string, error) {
    // Determine source preferences
    source, _ := cmd.Flags().GetString("templates-source")
    templatesDir, _ := cmd.Flags().GetString("templates-dir")
    baseOverride, _ := cmd.Flags().GetString("templates-base-url")

    // Gather names
    names := []string{}
    if source == "api" {
        if strings.TrimSpace(teamKey) == "" { return "", errors.New("--team is required with --templates-source=api") }
        t, err := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
        if err != nil { return "", err }
        if t == nil { return "", fmt.Errorf("team with key %s not found", teamKey) }
        items, err := client.ListIssueTemplatesForTeam(t.ID)
        if err != nil { return "", err }
        for _, it := range items { names = append(names, it.Name) }
    } else {
        // Local
        dirs := templateSearchDirs(templatesDir)
        seen := map[string]struct{}{}
        for _, dir := range dirs {
            entries, err := os.ReadDir(dir)
            if err != nil { continue }
            for _, e := range entries {
                if e.IsDir() { continue }
                n := e.Name()
                if strings.HasSuffix(strings.ToLower(n), ".md") {
                    base := strings.TrimSuffix(n, ".md")
                    if _, ok := seen[base]; !ok { seen[base] = struct{}{}; names = append(names, base) }
                }
            }
        }
        // Remote index
        if base := templateBaseURL(baseOverride); base != "" {
            if content, err := fetchURL(joinURL(base, "index.json")); err == nil {
                var arr []string
                var obj struct{ Templates []string `json:"templates"` }
                if json.Unmarshal([]byte(content), &arr) == nil {
                    for _, n := range arr { n = strings.TrimSpace(n); if n == "" { continue }; names = append(names, n) }
                } else if json.Unmarshal([]byte(content), &obj) == nil {
                    for _, n := range obj.Templates { n = strings.TrimSpace(n); if n == "" { continue }; names = append(names, n) }
                }
            }
        }
    }
    if len(names) == 0 {
        return "", errors.New("no templates available to choose from")
    }
    // Prompt
    fmt.Println("Select a template:")
    for i, n := range names { fmt.Printf("  %d) %s\n", i+1, n) }
    fmt.Print("> ")
    rdr := bufio.NewReader(os.Stdin)
    line, _ := rdr.ReadString('\n')
    choice := strings.TrimSpace(line)
    // Try number
    if idx, err := strconv.Atoi(choice); err == nil {
        if idx >= 1 && idx <= len(names) { return names[idx-1], nil }
    }
    // Try exact match by name
    for _, n := range names { if strings.EqualFold(n, choice) { return n, nil } }
    // Fallback: return input as-is to allow manual entry
    return choice, nil
}

// promptMultilineDescription asks the user for a multi-line description terminated by a single '.' on its own line.
func promptMultilineDescription() string {
    fmt.Println("Enter issue description. End with a single '.' on its own line:")
    rdr := bufio.NewReader(os.Stdin)
    var lines []string
    for {
        line, _ := rdr.ReadString('\n')
        line = strings.TrimRight(line, "\r\n")
        if line == "." { break }
        lines = append(lines, line)
    }
    return strings.TrimSpace(strings.Join(lines, "\n"))
}

// promptLine prints a prompt and returns a single line input (trimmed)
func promptLine(label string) string {
    fmt.Print(label)
    rdr := bufio.NewReader(os.Stdin)
    line, _ := rdr.ReadString('\n')
    return strings.TrimSpace(line)
}

// promptChoice prints a label and numbered options; returns the chosen option string.
func promptChoice(label string, options []string) string {
    if len(options) == 0 { return "" }
    fmt.Println(label + ":")
    for i, opt := range options { fmt.Printf("  %d) %s\n", i+1, opt) }
    fmt.Print("> ")
    rdr := bufio.NewReader(os.Stdin)
    line, _ := rdr.ReadString('\n')
    choice := strings.TrimSpace(line)
    if idx, err := strconv.Atoi(choice); err == nil {
        if idx >= 1 && idx <= len(options) { return options[idx-1] }
    }
    // fallback to matching by text
    for _, opt := range options { if strings.EqualFold(opt, choice) { return opt } }
    return options[0]
}

// promptChoiceStrict enforces valid selection; allowSkip adds a "(skip)" option.
func promptChoiceStrict(label string, options []string, allowSkip bool) string {
    opts := append([]string{}, options...)
    if allowSkip { opts = append([]string{"(skip)"}, opts...) }
    for {
        choice := promptChoice(label, opts)
        if allowSkip && (strings.EqualFold(choice, "(skip)") || strings.EqualFold(choice, "skip")) { return "" }
        for _, opt := range opts { if strings.EqualFold(opt, choice) { return opt } }
        fmt.Println("Invalid selection. Please choose a number or matching text.")
    }
}

// promptMultiSelect lets user select multiple items by entering comma-separated indexes or names.
func promptMultiSelect(label string, options []string) []string {
    fmt.Println(label)
    for i, opt := range options { fmt.Printf("  %d) %s\n", i+1, opt) }
    fmt.Print("> ")
    rdr := bufio.NewReader(os.Stdin)
    line, _ := rdr.ReadString('\n')
    line = strings.TrimSpace(line)
    if line == "" { return nil }
    parts := strings.Split(line, ",")
    var out []string
    for _, p := range parts {
        v := strings.TrimSpace(p)
        if v == "" { continue }
        if idx, err := strconv.Atoi(v); err == nil {
            if idx >= 1 && idx <= len(options) { out = append(out, options[idx-1]) }
            continue
        }
        // match by name
        for _, opt := range options { if strings.EqualFold(opt, v) { out = append(out, opt); break } }
    }
    return out
}

// promptYesNo asks a yes/no question; defaultYes controls default on empty input.
func promptYesNo(label string, defaultYes bool) bool {
    fmt.Print(label)
    rdr := bufio.NewReader(os.Stdin)
    line, _ := rdr.ReadString('\n')
    v := strings.TrimSpace(strings.ToLower(line))
    if v == "" { return defaultYes }
    return v == "y" || v == "yes"
}

// openInEditor opens $VISUAL or $EDITOR (falls back to vi) to edit text; returns the updated content.
func openInEditor(initial string) (string, error) {
    tmp, err := os.CreateTemp("", "linear-cli-*.md")
    if err != nil { return "", err }
    path := tmp.Name()
    _ = tmp.Close()
    if err := os.WriteFile(path, []byte(initial), 0o600); err != nil { return "", err }
    editor := strings.TrimSpace(os.Getenv("VISUAL"))
    if editor == "" { editor = strings.TrimSpace(os.Getenv("EDITOR")) }
    if editor == "" { editor = "vi" }
    cmd := exec.Command(editor, path)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil { return "", err }
    b, err := os.ReadFile(path)
    if err != nil { return "", err }
    _ = os.Remove(path)
    return string(b), nil
}

// autoLoadTemplateByKind tries API, then remote base, then local for a given kind (e.g., "feature", "bug", "spike").
func autoLoadTemplateByKind(kind string, cmd *cobra.Command, client *api.Client, teamID string) (string, bool) {
    name := strings.ToLower(strings.TrimSpace(kind))
    if name == "" { return "", false }
    // Try API
    if teamID != "" {
        if tpl, err := client.IssueTemplateByNameForTeam(teamID, name); err == nil && tpl != nil && strings.TrimSpace(tpl.Description) != "" { return tpl.Description, true }
    }
    // Try remote/local
    templatesDir, _ := cmd.Flags().GetString("templates-dir")
    baseOverride, _ := cmd.Flags().GetString("templates-base-url")
    if raw, err := loadTemplateContent(name, templatesDir, baseOverride); err == nil && strings.TrimSpace(raw) != "" { return raw, true }
    return "", false
}

// templateBaseURL resolves the remote templates base URL from flag/env.
// Precedence: explicit override -> $LINEAR_TEMPLATES_BASE_URL -> empty
func templateBaseURL(override string) string {
    if u := strings.TrimSpace(override); u != "" { return u }
    if u := strings.TrimSpace(os.Getenv("LINEAR_TEMPLATES_BASE_URL")); u != "" { return u }
    return ""
}

// joinURL concatenates base and path without duplicating slashes.
func joinURL(base string, path string) string {
    b := strings.TrimRight(base, "/")
    p := strings.TrimLeft(path, "/")
    return b + "/" + p
}

// fetchURL performs a simple GET and returns body as string if 200 OK.
func fetchURL(url string) (string, error) {
    req, err := http.NewRequest("GET", url, nil)
    if err != nil { return "", err }
    // Best effort: identify CLI in UA
    req.Header.Set("User-Agent", "linear-cli/0 (+https://github.com/nik")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return "", fmt.Errorf("GET %s: %s", url, resp.Status)
    }
    b, err := io.ReadAll(resp.Body)
    if err != nil { return "", err }
    return string(b), nil
}

// fillTemplate replaces {{PLACEHOLDER}} tokens in the template with values from vars.
// If interactive is true, it prompts for any missing placeholders on stdin.
// If failOnMissing is true and any placeholders remain unresolved, returns an error.
func fillTemplate(tpl string, vars map[string]string, interactive bool, failOnMissing bool) (string, error) {
    content := tpl
    // Find all placeholders of the form {{SOMETHING}} or {{SOMETHING|Prompt text...}}
    re := regexp.MustCompile(`\{\{\s*([A-Za-z0-9_\-]+)(?:\|([^}]+))?\s*\}\}`)
    // Build a set of unique keys
    seen := make(map[string]struct{})
    matches := re.FindAllStringSubmatch(content, -1)
    prompts := make(map[string]string)
    for _, m := range matches {
        if len(m) >= 2 {
            seen[m[1]] = struct{}{}
            if len(m) >= 3 && strings.TrimSpace(m[2]) != "" { prompts[m[1]] = strings.TrimSpace(m[2]) }
        }
    }
    missing := make([]string, 0)
    for key := range seen {
        if _, ok := vars[key]; !ok {
            missing = append(missing, key)
        }
    }
    if interactive && len(missing) > 0 {
        rdr := bufio.NewReader(os.Stdin)
        for _, key := range missing {
            prompt := key
            if p, ok := prompts[key]; ok { prompt = fmt.Sprintf("%s\n> ", p) } else { prompt = prompt + ": " }
            fmt.Print(prompt)
            line, _ := rdr.ReadString('\n')
            vars[key] = strings.TrimSpace(line)
        }
        missing = missing[:0]
        for key := range seen { if _, ok := vars[key]; !ok { missing = append(missing, key) } }
    }
    if failOnMissing && len(missing) > 0 {
        return "", fmt.Errorf("missing values for: %s", strings.Join(missing, ", "))
    }
    // Replace all placeholders. Keep unknowns as-is if not failing
    content = re.ReplaceAllStringFunc(content, func(s string) string {
        m := re.FindStringSubmatch(s)
        if len(m) >= 2 {
            if v, ok := vars[m[1]]; ok { return v }
        }
        return s
    })
    return content, nil
}

// hasTemplatePlaceholders reports whether the template contains any {{KEY}} tokens
func hasTemplatePlaceholders(tpl string) bool {
    re := regexp.MustCompile(`\{\{\s*([A-Za-z0-9_\-]+)(?:\|[^}]+)?\s*\}\}`)
    return re.MatchString(tpl)
}

// promptSectionsFromTemplate extracts markdown-style sections (lines ending with ':' or '## Heading')
// and prompts the user to fill each one, composing a structured description.
func promptSectionsFromTemplate(tpl string) string {
    lines := strings.Split(tpl, "\n")
    type section struct{ title string }
    var sections []section
    for _, ln := range lines {
        s := strings.TrimSpace(ln)
        if s == "" { continue }
        // Headings like "Summary" or "Context" on their own line
        // or markdown headings
        if s == strings.ToUpper(string(s[0]))+s[1:] && !strings.Contains(s, " ") == false { /* passthrough */ }
        if strings.HasPrefix(s, "## ") || strings.HasSuffix(s, ":") || isStandaloneHeading(s) {
            title := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(s, "## ")), ":")
            if title != "" { sections = append(sections, section{title: title}) }
        }
    }
    if len(sections) == 0 {
        return promptMultilineBlock("Description")
    }
    var b strings.Builder
    for i, sec := range sections {
        if i > 0 { b.WriteString("\n\n") }
        b.WriteString("## ")
        b.WriteString(sec.title)
        b.WriteString("\n")
        b.WriteString(promptMultilineBlock(sec.title))
    }
    return strings.TrimSpace(b.String())
}

// buildDescriptionFromTemplate chooses the best interactive strategy to produce a description
// from a template: placeholder prompting when tokens exist, otherwise section-by-section prompts.
func buildDescriptionFromTemplate(tpl string, vars map[string]string, interactive bool, failOnMissing bool) (string, error) {
    if strings.TrimSpace(tpl) == "" {
        if interactive { return promptMultilineBlock("Description"), nil }
        return "", nil
    }
    if hasTemplatePlaceholders(tpl) {
        return fillTemplate(tpl, vars, interactive, failOnMissing)
    }
    if interactive {
        return promptSectionsFromTemplate(tpl), nil
    }
    // Non-interactive, no placeholders: return raw body
    return tpl, nil
}

// isStandaloneHeading heuristically detects single-line headings like "Summary", "Context",
// "Requirements", "Definition of Done".
func isStandaloneHeading(s string) bool {
    ss := strings.TrimSpace(s)
    if strings.Contains(ss, " ") { return false }
    if len(ss) < 3 { return false }
    // Title-case word without trailing punctuation
    return strings.ToUpper(string(ss[0]))+ss[1:] == ss && !strings.HasSuffix(ss, ":")
}

// promptMultilineBlock prompts the user for a multi-line block with a clear label; end on empty line.
func promptMultilineBlock(label string) string {
    fmt.Printf("%s (finish with an empty line):\n", label)
    rdr := bufio.NewReader(os.Stdin)
    var lines []string
    for {
        line, _ := rdr.ReadString('\n')
        line = strings.TrimRight(line, "\r\n")
        if strings.TrimSpace(line) == "" { break }
        lines = append(lines, line)
    }
    return strings.TrimSpace(strings.Join(lines, "\n"))
}

// parseTitlePrefixAndStrip allows templates to declare a title prefix on the first line like:
// Title-Prefix: Feat:
// The line is removed from the template body and the prefix returned.
func parseTitlePrefixAndStrip(tpl string) (prefix string, body string) {
    lines := strings.Split(tpl, "\n")
    if len(lines) == 0 { return "", tpl }
    first := strings.TrimSpace(lines[0])
    if strings.HasPrefix(strings.ToLower(first), "title-prefix:") {
        val := strings.TrimSpace(strings.TrimPrefix(first, "title-prefix:"))
        return val, strings.Join(lines[1:], "\n")
    }
    return "", tpl
}

// gatherVars merges vars from CLI kv pairs and optional JSON file
func gatherVars(kvs []string, file string) (map[string]string, error) {
    out := make(map[string]string)
    for _, kv := range kvs {
        if kv == "" { continue }
        parts := strings.SplitN(kv, "=", 2)
        if len(parts) != 2 { return nil, fmt.Errorf("invalid --var, expect key=value: %s", kv) }
        out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
    }
    if strings.TrimSpace(file) != "" {
        path := file
        if strings.HasPrefix(path, "~") {
            if home, err := os.UserHomeDir(); err == nil {
                path = filepath.Join(home, strings.TrimPrefix(path, "~"))
            }
        }
        b, err := os.ReadFile(path)
        if err != nil { return nil, err }
        var m map[string]string
        if err := json.Unmarshal(b, &m); err != nil { return nil, err }
        for k, v := range m { out[k] = v }
    }
    return out, nil
}

// fillTemplateFromDescription intelligently fills template sections from a provided description
func fillTemplateFromDescription(templateContent, userDescription string) string {
	// Parse the template to find sections
	sections := parseTemplateSections(templateContent)
	if len(sections) == 0 {
		return userDescription // No template structure, just use user description
	}
	
	// Smart filling: try to extract relevant parts from user description
	filled := templateContent
	
	// Simple heuristic: use the first sentence(s) for Summary
	sentences := strings.Split(strings.TrimSpace(userDescription), ". ")
	if len(sentences) > 0 {
		summary := sentences[0]
		if !strings.HasSuffix(summary, ".") { summary += "." }
		filled = strings.Replace(filled, "One or two sentences describing what this issue is and why it matters.", summary, 1)
	}
	
	// Use remaining content for Context if we have more than one sentence
	if len(sentences) > 1 {
		context := strings.Join(sentences[1:], ". ")
		if strings.TrimSpace(context) != "" {
			filled = strings.Replace(filled, "Relevant background, links, or reasoning behind the request.", context, 1)
		}
	}
	
	// For Requirements and DOD, provide helpful defaults that can be edited
	filled = strings.Replace(filled, "- [ ] Requirement 1\n- [ ] Requirement 2\n- [ ] (Optional) Stretch goal", 
		"- [ ] Implement core functionality\n- [ ] Add appropriate tests\n- [ ] Update documentation", 1)
	
	filled = strings.Replace(filled, "Clear outcome that marks this task as complete.", 
		"Feature is implemented, tested, and ready for production use.", 1)
	
	return filled
}

// promptTemplateInteractively prompts user to fill each template section
func promptTemplateInteractively(templateContent string) string {
	sections := parseTemplateSections(templateContent)
	if len(sections) == 0 {
		// No structured template, just prompt for description
		return promptMultilineBlock("Description")
	}
	
	filled := templateContent
	
	// Prompt for each section
	if strings.Contains(filled, "One or two sentences describing what this issue is and why it matters.") {
		summary := promptLine("Summary (1-2 sentences): ")
		if strings.TrimSpace(summary) != "" {
			filled = strings.Replace(filled, "One or two sentences describing what this issue is and why it matters.", summary, 1)
		}
	}
	
	if strings.Contains(filled, "Relevant background, links, or reasoning behind the request.") {
		context := promptMultilineBlock("Context")
		if strings.TrimSpace(context) != "" {
			filled = strings.Replace(filled, "Relevant background, links, or reasoning behind the request.", context, 1)
		}
	}
	
	if strings.Contains(filled, "- [ ] Requirement 1\n- [ ] Requirement 2\n- [ ] (Optional) Stretch goal") {
		requirements := promptMultilineBlock("Requirements (one per line, use - [ ] format)")
		if strings.TrimSpace(requirements) != "" {
			filled = strings.Replace(filled, "- [ ] Requirement 1\n- [ ] Requirement 2\n- [ ] (Optional) Stretch goal", requirements, 1)
		}
	}
	
	if strings.Contains(filled, "Clear outcome that marks this task as complete.") {
		dod := promptLine("Definition of Done: ")
		if strings.TrimSpace(dod) != "" {
			filled = strings.Replace(filled, "Clear outcome that marks this task as complete.", dod, 1)
		}
	}
	
	return filled
}

// parseTemplateSections extracts section headers from template content
func parseTemplateSections(content string) []string {
	var sections []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "## ") {
			sections = append(sections, strings.TrimPrefix(strings.TrimPrefix(line, "### "), "## "))
		}
	}
	return sections
}

// fillTemplateSectionsDynamically fills template sections using provided key-value pairs
func fillTemplateSectionsDynamically(templateContent string, sections map[string]string) string {
	filled := templateContent
	
	// Process each section that we have content for
	for sectionName, content := range sections {
		filled = fillSingleSection(filled, sectionName, content)
	}
	
	return filled
}

// getSectionKeys returns the keys from a sections map
func getSectionKeys(sections map[string]string) []string {
	keys := make([]string, 0, len(sections))
	for key := range sections {
		keys = append(keys, key)
	}
	return keys
}

// fillSingleSection fills a single section in the template content
func fillSingleSection(templateContent, sectionName, content string) string {
	lines := strings.Split(templateContent, "\n")
	
	for i, line := range lines {
		line = strings.TrimSpace(line)
		// Check if this is the section header we're looking for
		if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "## ") {
			headerSectionName := strings.TrimPrefix(strings.TrimPrefix(line, "### "), "## ")
			
			if headerSectionName == sectionName {
				// Find the next section or end of content
				nextSectionIdx := len(lines)
				for j := i + 1; j < len(lines); j++ {
					nextLine := strings.TrimSpace(lines[j])
					if strings.HasPrefix(nextLine, "### ") || strings.HasPrefix(nextLine, "## ") {
						nextSectionIdx = j
						break
					}
				}
				
				// Replace the content between this section and the next
				newLines := make([]string, 0, len(lines))
				newLines = append(newLines, lines[:i+1]...) // Include section header
				newLines = append(newLines, "")              // Empty line
				newLines = append(newLines, content)         // New content
				newLines = append(newLines, "")              // Empty line
				newLines = append(newLines, lines[nextSectionIdx:]...) // Rest of content
				
				return strings.Join(newLines, "\n")
			}
		}
	}
	
	// Section not found, return unchanged
	return templateContent
}

// buildExampleSections creates an example --sections flag value from section names
func buildExampleSections(sections []string) string {
	if len(sections) == 0 { return "" }
	examples := make([]string, 0, len(sections))
	for _, section := range sections {
		examples = append(examples, fmt.Sprintf("%s='Your %s content'", section, strings.ToLower(section)))
	}
	return strings.Join(examples, " ")
}

// createIssueAIFriendly handles AI-optimized issue creation with auto-discovery and seamless workflow
func createIssueAIFriendly(client *api.Client, teamKey, templateName, title string, sections map[string]string, cmd *cobra.Command) error {
	// Get team info
	team, err := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
	if err != nil {
		return fmt.Errorf("failed to find team %s: %w", teamKey, err)
	}
	if team == nil {
		return fmt.Errorf("team with key %s not found", teamKey)
	}

	// Try to get template info from local cache first
	templateInfo, _, err := GetLocalTemplate(teamKey, templateName)
	if err != nil {
		// Local template not found - auto-sync and try again
		fmt.Printf("🔄 Template not cached locally, auto-syncing templates for team %s...\n", teamKey)
		
		// Get templates directory
		templatesDir, err := getTemplatesDir()
		if err != nil {
			return fmt.Errorf("failed to access templates directory: %w", err)
		}

		// Load or create metadata
		metadata, err := loadTemplateMetadata(templatesDir)
		if err != nil {
			metadata = &TemplateMetadata{
				Templates: make(map[string]TeamTemplates),
			}
		}

		// Auto-sync this team's templates
		syncResult, err := syncTeamTemplatesIntelligent(client, *team, templatesDir, metadata)
		if err != nil {
			return fmt.Errorf("failed to auto-sync templates: %w", err)
		}

		// Save metadata
		metadata.LastSync = time.Now()
		_ = saveTemplateMetadata(templatesDir, metadata) // Best effort

		if syncResult.SkipReason != "" {
			fmt.Printf("   %s\n", syncResult.SkipReason)
		} else {
			fmt.Printf("   %s\n", syncResult.SyncSummary)
		}

		// Try to get template info again
		templateInfo, _, err = GetLocalTemplate(teamKey, templateName)
		if err != nil {
			// Still not found - provide helpful error with available templates
			templates, listErr := GetLocalTemplatesForTeam(teamKey)
			if listErr != nil {
				return fmt.Errorf("template '%s' not found and failed to list available templates: %w", templateName, err)
			}
			
			availableNames := make([]string, len(templates))
			for i, t := range templates {
				availableNames[i] = t.Name
			}
			
			return fmt.Errorf("template '%s' not found for team %s. Available templates: %s", 
				templateName, teamKey, strings.Join(availableNames, ", "))
		}
	}

	fmt.Printf("📋 Using template: %s (ID: %s)\n", templateInfo.Name, templateInfo.ID)

	// Pre-fill template sections using local template content
	var prefilledDescription string
	if len(sections) > 0 {
		fmt.Printf("📝 Pre-filling %d template sections...\n", len(sections))
		
		// Get the local template content and fill sections
		_, localTemplateContent, err := GetLocalTemplate(teamKey, templateName)
		if err != nil {
			return fmt.Errorf("failed to get local template content: %w", err)
		}
		
		prefilledDescription = fillTemplateSectionsDynamically(localTemplateContent, sections)
		fmt.Printf("   ✓ Template sections pre-filled\n")
	}

	// Create issue with server-side template application and pre-filled description
	createInput := api.IssueCreateInput{
		TeamID:     team.ID,
		TemplateID: templateInfo.ID,
		Title:      title,
		Priority:   &[]int{3}[0], // Default to Medium priority
	}
	
	// If we have pre-filled content, use it as the description
	if prefilledDescription != "" {
		createInput.Description = prefilledDescription
	}

	created, err := client.CreateIssueAdvanced(createInput)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}

	fmt.Printf("✅ Created issue: %s\n", created.Identifier)
	if len(sections) > 0 {
		fmt.Printf("   ✓ %d template sections filled\n", len(sections))
	}

	// Output result
	p := printer(cmd)
	if p.JSONEnabled() {
		return p.PrintJSON(map[string]interface{}{
			"success":    true,
			"id":         created.ID,
			"identifier": created.Identifier,
			"title":      created.Title,
			"url":        created.URL,
			"template": map[string]interface{}{
				"name": templateInfo.Name,
				"id":   templateInfo.ID,
			},
			"sections_filled": len(sections),
			"auto_synced":     err != nil, // Whether we had to auto-sync
		})
	}

	fmt.Printf("\n🎉 Issue created successfully!\n")
	fmt.Printf("   Title: %s\n", created.Title)
	fmt.Printf("   URL: %s\n", created.URL)
	fmt.Printf("   Template: %s\n", templateInfo.Name)
	if len(sections) > 0 {
		fmt.Printf("   Sections filled: %d\n", len(sections))
	}
	
	return nil
}
