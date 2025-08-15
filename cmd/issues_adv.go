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

var issuesTemplateListCmd = &cobra.Command{
    Use:   "list",
    Short: "List available templates",
    RunE: func(cmd *cobra.Command, args []string) error {
        override, _ := cmd.Flags().GetString("templates-dir")
        baseOverride, _ := cmd.Flags().GetString("templates-base-url")
        source, _ := cmd.Flags().GetString("templates-source")
        teamKey, _ := cmd.Flags().GetString("team")
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
        var raw string
        var err error
        if source == "api" {
            cfg, _ := config.Load()
            if cfg.APIKey == "" { return errors.New("not authenticated. run 'linear-cli auth login'") }
            client := api.NewClient(cfg.APIKey)
            // Try by id first
            if tpl, e := client.IssueTemplateByID(args[0]); e == nil && tpl != nil {
                raw = tpl.Description
            } else {
                if strings.TrimSpace(teamKey) == "" { return errors.New("--team is required to resolve template by name with --templates-source=api") }
                t, errT := client.TeamByKey(strings.ToUpper(strings.TrimSpace(teamKey)))
                if errT != nil { return errT }
                if t == nil { return fmt.Errorf("team with key %s not found", teamKey) }
                tpl, errN := client.IssueTemplateByNameForTeam(t.ID, args[0])
                if errN != nil { return errN }
                if tpl == nil { return fmt.Errorf("template '%s' not found for team %s", args[0], teamKey) }
                raw = tpl.Description
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
        if p.JSONEnabled() { return p.PrintJSON(map[string]any{"content": rendered}) }
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
    Long: `Create a new issue. Provide --team (key like ENG). Other fields are prompted interactively by default.

Template-driven creation
 - Use --template <name|path|url> to load a markdown template. Named templates resolve from local dirs or a remote base URL
   - Local search order: --templates-dir, $LINEAR_TEMPLATES_DIR, UserConfigDir/linear/templates, ~/.config/linear/templates
   - Remote base: --templates-base-url or $LINEAR_TEMPLATES_BASE_URL. Names resolve to <base>/<name>.md
- Title prefix: If the first line of the template is 'Title-Prefix: <prefix>' it will be prepended to --title automatically
- Placeholders: Write {{KEY}} or {{KEY|Prompt text...}} anywhere in the template body
  - Interactive is the default when using --template without --description; disable with --no-interactive
  - With --interactive, you will be prompted for each missing KEY, showing the prompt text when provided
  - Use --var KEY=VALUE (repeatable) and/or --vars-file file.json to prefill values
  - Use --fail-on-missing to error out if any placeholders remain unresolved
- Preview is automatic when prefill variables are provided (disable with --no-preview). Use -y/--yes to submit after preview

Examples of placeholders
  {{SUMMARY|One or two sentences describing what this issue is and why it matters.}}
  {{CONTEXT|Relevant background, links, or reasoning behind the request.}}
  {{REQUIREMENTS|List the key requirements.}}
  {{DOD|Clear outcome that marks this task as complete.}}`,
    Example: `  # Create a bug using a named template and team key
  linear-cli issues create --title "Crash on save" --team ENG --template bug --label bug --priority 2

  # Create a feature in a project by name using a template file (preview first)
  linear-cli issues create --title "Dark mode" --project "Website" --template ~/.config/linear/templates/feature.md --preview

  # Interactive walkthrough filling placeholders
  linear-cli issues create --title "Dark mode" --team ENG --template feature -i

  # Provide variables explicitly (no prompts)
  linear-cli issues create --title "Dark mode" --team ENG --template feature \
    --var SUMMARY="Add dark theme for night-time usability" \
    --var CONTEXT="Many users work in low-light settings" \
    --var REQUIREMENTS=$'Toggle in settings\\nAuto-detect system theme' \
    --var DOD="All screens match dark palette; accessibility checks pass"`,
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
        // Interactive is the default unless explicitly disabled
        interactive := interactiveFlag
        if !noInteractive && !cmd.Flags().Changed("interactive") {
            interactive = true
        }
        preview := previewFlag
        if varsProvided && !noPreview && !cmd.Flags().Changed("preview") {
            preview = true
        }

        // If user requested interactive but provided no template or description, offer to pick a template
        if !interactive && strings.TrimSpace(templateName) == "" && strings.TrimSpace(description) == "" {
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
        if strings.TrimSpace(description) == "" && strings.TrimSpace(templateName) != "" {
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
        }

        // As a final guard in interactive mode, if description is still empty, prompt for a multiline description
        if interactive && strings.TrimSpace(description) == "" {
            description = promptMultilineDescription()
        }
        var projectID string
        var teamID string
        if project != "" {
            pr, err := client.ResolveProject(project)
            if err != nil { return err }
            if pr == nil { return fmt.Errorf("project '%s' not found", project) }
            projectID = pr.ID
            if pr.TeamID != "" { teamID = pr.TeamID }
        }
        if teamKey != "" {
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
        // If interactive and still missing, walk through all fields
        if interactive {
            // Title
            if strings.TrimSpace(title) == "" { title = promptLine("Title: ") }
            // Kind -> optional auto-label mapping
            kind := promptChoiceStrict("Issue type", []string{"Feature", "Bug", "Spike"}, false)
            // Attempt to auto-load template by kind if no description/template provided
            if strings.TrimSpace(description) == "" && strings.TrimSpace(templateName) == "" {
                preferAPI := client.SupportsIssueTemplates()
                if tplContent, ok := autoLoadTemplateByKind(kind, cmd, client, teamID); ok {
                    if prefix, body := parseTitlePrefixAndStrip(tplContent); prefix != "" {
                        if !strings.HasPrefix(strings.TrimSpace(title), prefix) { title = strings.TrimSpace(prefix + " " + title) }
                        tplContent = body
                    }
                    vars, _ := gatherVars(varsKVs, varsFile)
                    if rendered, err := fillTemplate(tplContent, vars, true, false); err == nil { description = rendered }
                } else {
                    // Fallback: allow user to pick a template interactively
                    if preferAPI { _ = cmd.Flags().Set("templates-source", "api") }
                    if tmpl, err := interactivePickTemplate(cmd, client, teamKey); err == nil && strings.TrimSpace(tmpl) != "" {
                        templatesDir, _ := cmd.Flags().GetString("templates-dir")
                        baseOverride, _ := cmd.Flags().GetString("templates-base-url")
                        if raw, err := loadTemplateContent(tmpl, templatesDir, baseOverride); err == nil {
                            if prefix, body := parseTitlePrefixAndStrip(raw); prefix != "" {
                                if !strings.HasPrefix(strings.TrimSpace(title), prefix) { title = strings.TrimSpace(prefix + " " + title) }
                                raw = body
                            }
                            vars, _ := gatherVars(varsKVs, varsFile)
                            if rendered, err := fillTemplate(raw, vars, true, false); err == nil { description = rendered }
                        }
                    }
                }
            }
            // Offer to add a label matching the kind if available
            if strings.TrimSpace(kind) != "" {
                if labels, err := client.ListIssueLabels(200); err == nil && len(labels) > 0 {
                    var kindLabelID string
                    for _, lb := range labels { if strings.EqualFold(lb.Name, kind) { kindLabelID = lb.ID; break } }
                    if kindLabelID != "" { labelIDs = append(labelIDs, kindLabelID) }
                }
            }
            // Priority
            if prioPtr == nil {
                prioStr := promptChoice("Priority", []string{"1", "2", "3", "4"})
                if v, err := strconv.Atoi(prioStr); err == nil { prioPtr = &v }
            }
            // Assignee from team members
            if assigneeID == "" && teamID != "" {
                members, _ := client.TeamMembers(teamID)
                if len(members) > 0 {
                    // Build options including (me)
                    var me *api.Viewer
                    if v, err := client.Viewer(); err == nil { me = v }
                    opts := []string{"(none)"}
                    idxByName := map[string]int{}
                    if me != nil { opts = append(opts, fmt.Sprintf("(me) %s", me.Name)) }
                    for i, u := range members { opts = append(opts, u.Name); idxByName[u.Name] = i }
                    pick := promptChoiceStrict("Assignee", opts, true)
                    if pick == "(none)" {
                        // leave empty
                    } else if me != nil && (strings.EqualFold(pick, "(me)") || strings.Contains(pick, me.Name)) {
                        assigneeID = me.ID
                    } else {
                        if i, ok := idxByName[pick]; ok { assigneeID = members[i].ID }
                    }
                }
            }
            // Project picker (from API, filtered by team)
            if projectID == "" {
                var projects []api.Project
                if teamID != "" { projects, _ = client.ListProjectsByTeam(teamID, 200) }
                if len(projects) == 0 { projects, _ = client.ListProjectsAll(200) }
                if len(projects) > 0 {
                    opts := make([]string, len(projects)+1)
                    opts[0] = "(none)"
                    for i, p := range projects { opts[i+1] = p.Name }
                    pick := promptChoice("Project", opts)
                    if pick != "(none)" {
                        for i, p := range projects { if p.Name == pick { projectID = projects[i].ID; break } }
                    }
                } else {
                    // fallback text entry
                    pName := strings.TrimSpace(promptLine("Project (leave blank for none): "))
                    if pName != "" { if pr, err := client.ResolveProject(pName); err == nil && pr != nil { projectID = pr.ID } }
                }
            }
            // Labels multi-select
            if labels, err := client.ListIssueLabels(200); err == nil && len(labels) > 0 {
                names := make([]string, len(labels))
                for i, lb := range labels { names[i] = lb.Name }
                picks := promptMultiSelect("Labels (comma-separated numbers or names, blank to skip)", names)
                if len(picks) > 0 {
                    // merge unique
                    existing := map[string]struct{}{}
                    for _, id := range labelIDs { existing[id] = struct{}{} }
                    for _, pick := range picks {
                        // match by name to id
                        for _, lb := range labels { if strings.EqualFold(lb.Name, pick) { if _, ok := existing[lb.ID]; !ok { labelIDs = append(labelIDs, lb.ID); existing[lb.ID] = struct{}{} } } }
                    }
                }
            }

            // Ensure description
            if strings.TrimSpace(description) == "" { description = promptMultilineDescription() }
            // Optional editor for final tweaks
            if promptYesNo("Open in editor to finalize description? (y/N): ", false) {
                if edited, err := openInEditor(description); err == nil { description = edited }
            }
        }

        // Final: create (include state if chosen earlier)
        // Re-prompt state if interactive and not set
        var chosenStateID string
        if interactive {
            states, _ := client.TeamStates(teamID)
            if len(states) > 0 {
                opts := make([]string, len(states))
                idByName := map[string]string{}
                for i, s := range states { opts[i] = s.Name; idByName[s.Name] = s.ID }
                pick := promptChoice("State", opts)
                if id, ok := idByName[pick]; ok { chosenStateID = id }
            }
        }
        created, err := client.CreateIssueAdvanced(api.IssueCreateInput{ProjectID: projectID, TeamID: teamID, StateID: chosenStateID, Title: title, Description: description, AssigneeID: assigneeID, LabelIDs: labelIDs, Priority: prioPtr})
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
    issuesTemplateCmd.AddCommand(issuesTemplateListCmd)
    issuesTemplateCmd.AddCommand(issuesTemplatePreviewCmd)

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
    issuesTemplatePreviewCmd.Flags().StringArray("var", nil, "Template variable assignment key=value (repeatable)")
    issuesTemplatePreviewCmd.Flags().String("vars-file", "", "JSON file with string key-value pairs for template variables")
    issuesTemplatePreviewCmd.Flags().String("templates-dir", "", "Override templates directory (default search: $LINEAR_TEMPLATES_DIR, UserConfigDir/linear/templates, ~/.config/linear/templates)")
    issuesTemplatePreviewCmd.Flags().String("templates-base-url", "", "Remote templates base URL (fallback: $LINEAR_TEMPLATES_BASE_URL). Names resolve to <base>/<name>.md")
    issuesTemplatePreviewCmd.Flags().String("templates-source", "auto", "Template source: auto|local|remote|api")
    issuesTemplatePreviewCmd.Flags().String("team", "", "Team key when using --templates-source=api")
    issuesTemplateListCmd.Flags().String("templates-dir", "", "Override templates directory (default search: $LINEAR_TEMPLATES_DIR, UserConfigDir/linear/templates, ~/.config/linear/templates)")
    issuesTemplateListCmd.Flags().String("templates-base-url", "", "Remote templates base URL (fallback: $LINEAR_TEMPLATES_BASE_URL). Listing reads <base>/index.json")
    issuesTemplateListCmd.Flags().String("templates-source", "auto", "Template source: auto|local|remote|api")
    issuesTemplateListCmd.Flags().String("team", "", "Team key when using --templates-source=api")
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
