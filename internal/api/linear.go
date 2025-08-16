package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
	apiKey     string
	endpoint   string
    allowedMutations map[string]struct{}
    supportsTemplates *bool
}

type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

type Viewer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Team struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Issue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StateName   string `json:"stateName"`
	URL         string `json:"url"`
}

func NewClient(apiKey string) *Client {
    endpoint := "https://api.linear.app/graphql"
    if v := os.Getenv("LINEAR_API_ENDPOINT"); strings.TrimSpace(v) != "" {
        endpoint = strings.TrimSpace(v)
    }
    return &Client{
        httpClient: &http.Client{Timeout: 15 * time.Second},
        apiKey:     apiKey,
        endpoint:   endpoint,
        allowedMutations: map[string]struct{}{
            "issueCreate": {},
            "issueUpdate": {},
            "commentCreate": {},
        },
    }
}

// SupportsIssueTemplates performs a lightweight introspection check and caches the result.
func (c *Client) SupportsIssueTemplates() bool {
    if c.supportsTemplates != nil { return *c.supportsTemplates }
    const q = `query{ __type(name:"IssueTemplate"){ name } }`
    var resp struct{ Type *struct{ Name string `json:"name"` } `json:"__type"` }
    err := c.do(q, nil, &resp)
    supported := (err == nil && resp.Type != nil && resp.Type.Name != "")
    c.supportsTemplates = &supported
    return supported
}

func (c *Client) do(query string, variables map[string]interface{}, out interface{}) error {
    // Guard: forbid delete/archive operations and enforce allowlist
    if isMutation(query) {
        if containsDangerousOperation(query) {
            return errors.New("operation rejected: delete/archive mutations are not allowed")
        }
        names := mutationSelectionNames(query)
        if len(names) == 0 {
            return errors.New("invalid mutation: no selections")
        }
        for _, n := range names {
            if _, ok := c.allowedMutations[n]; !ok {
                return fmt.Errorf("mutation '%s' is not allowed", n)
            }
        }
    }

    payload := gqlRequest{Query: query, Variables: variables}
    buf, err := json.Marshal(payload)
    if err != nil { return err }

    var resp *http.Response
    for attempt := 0; attempt < 4; attempt++ {
        req, err := http.NewRequest("POST", c.endpoint, bytes.NewReader(buf))
        if err != nil { return err }
        req.Header.Set("Content-Type", "application/json")
        // Linear expects raw API key in the Authorization header
        req.Header.Set("Authorization", c.apiKey)

        resp, err = c.httpClient.Do(req)
        if err != nil {
            if attempt == 3 { return err }
            backoffSleep(attempt)
            continue
        }
        if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
            ra := resp.Header.Get("Retry-After")
            resp.Body.Close()
            sleepForRetryAfterOrBackoff(ra, attempt)
            continue
        }
        break
    }
    if resp == nil { return errors.New("no response from Linear API") }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 {
        // Try to decode GraphQL errors for a clearer message, otherwise include body text
        var gr gqlResponse
        dec := json.NewDecoder(resp.Body)
        if err := dec.Decode(&gr); err == nil && (len(gr.Errors) > 0 || len(gr.Data) > 0) {
            if len(gr.Errors) > 0 {
                return fmt.Errorf("linear api error: %s: %s", resp.Status, gr.Errors[0].Message)
            }
            return fmt.Errorf("linear api error: %s", resp.Status)
        }
        // Fallback: read raw body
        // Note: resp.Body has been partially read by decoder above only if it succeeded; otherwise we read remaining.
        // To be robust, we re-issue the request body content from original buffer in future improvements.
        var raw map[string]any
        _ = json.NewDecoder(resp.Body).Decode(&raw)
        return fmt.Errorf("linear api error: %s", resp.Status)
    }
    var gr gqlResponse
    if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil { return err }
    if len(gr.Errors) > 0 { return errors.New(gr.Errors[0].Message) }
    if out != nil && len(gr.Data) > 0 { return json.Unmarshal(gr.Data, out) }
    return nil
}

var (
    reMutation = regexp.MustCompile(`(?is)\bmutation\b`)
    reDelete   = regexp.MustCompile(`(?is)\b(delete|archive)\b`)
    reSelBlock = regexp.MustCompile(`(?is)mutation[^{]*\{([^}]*)\}`)
)

func isMutation(q string) bool { return reMutation.MatchString(q) }
func containsDangerousOperation(q string) bool { return reDelete.MatchString(q) }
func mutationSelectionNames(q string) []string {
    m := reSelBlock.FindStringSubmatch(q)
    if len(m) < 2 { return nil }
    block := m[1]
    lines := strings.Split(block, "\n")
    var names []string
    for _, line := range lines {
        s := strings.TrimSpace(line)
        if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "...") { continue }
        // Only treat ':' as an alias separator if it appears before any '(' '{' or space
        if idx := strings.Index(s, ":"); idx >= 0 {
            stopAt := len(s)
            if p := strings.IndexAny(s, "({ "); p >= 0 { stopAt = p }
            if idx < stopAt {
                s = strings.TrimSpace(s[idx+1:])
            }
        }
        for i, r := range s {
            if r == '(' || r == '{' || r == ' ' { s = s[:i]; break }
        }
        if s != "" { names = append(names, s) }
    }
    return names
}

func backoffSleep(attempt int) { time.Sleep(time.Duration(250*(1<<attempt)) * time.Millisecond) }
func sleepForRetryAfterOrBackoff(retryAfter string, attempt int) {
    if retryAfter == "" { backoffSleep(attempt); return }
    if d, err := time.ParseDuration(retryAfter + "s"); err == nil { time.Sleep(d); return }
    if t, err := time.Parse(time.RFC1123, retryAfter); err == nil {
        if dur := time.Until(t); dur > 0 { time.Sleep(dur); return }
    }
    backoffSleep(attempt)
}

func (c *Client) Viewer() (*Viewer, error) {
	const q = `query { viewer { id name email } }`
	var resp struct {
		Viewer Viewer `json:"viewer"`
	}
	if err := c.do(q, nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Viewer, nil
}

func (c *Client) TeamByKey(key string) (*Team, error) {
	const q = `query($key:String!){ teams(filter:{ key:{ eq:$key } }, first:1){ nodes{ id key name } } }`
	var resp struct {
		Teams struct {
			Nodes []Team `json:"nodes"`
		} `json:"teams"`
	}
	if err := c.do(q, map[string]interface{}{"key": key}, &resp); err != nil {
		return nil, err
	}
	if len(resp.Teams.Nodes) == 0 {
		return nil, nil
	}
	return &resp.Teams.Nodes[0], nil
}

func (c *Client) ListIssues(limit int, teamID string) ([]Issue, error) {
	if limit <= 0 {
		limit = 10
	}
    if teamID == "" {
        const q = `query($first:Int!){ issues(first:$first){ nodes{ id identifier title description url state{ name } } } }`
		var resp struct {
			Issues struct {
				Nodes []struct {
					ID         string `json:"id"`
					Identifier string `json:"identifier"`
					Title      string `json:"title"`
					Description string `json:"description"`
					URL        string `json:"url"`
					State      struct{ Name string `json:"name"` } `json:"state"`
				} `json:"nodes"`
			} `json:"issues"`
		}
		if err := c.do(q, map[string]interface{}{"first": limit}, &resp); err != nil {
			return nil, err
		}
		issues := make([]Issue, 0, len(resp.Issues.Nodes))
		for _, n := range resp.Issues.Nodes {
            issues = append(issues, Issue{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name})
		}
		return issues, nil
	}
    const q = `query($first:Int!,$teamId:String!){ issues(first:$first, filter:{ team: { id: { eq:$teamId } } }){ nodes{ id identifier title description url state{ name } } } }`
	var resp struct {
		Issues struct {
			Nodes []struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Title      string `json:"title"`
				Description string `json:"description"`
				URL        string `json:"url"`
				State      struct{ Name string `json:"name"` } `json:"state"`
			} `json:"nodes"`
		} `json:"issues"`
	}
	if err := c.do(q, map[string]interface{}{"first": limit, "teamId": teamID}, &resp); err != nil {
		return nil, err
	}
	issues := make([]Issue, 0, len(resp.Issues.Nodes))
	for _, n := range resp.Issues.Nodes {
        issues = append(issues, Issue{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name})
	}
	return issues, nil
}

func (c *Client) IssueByID(id string) (*Issue, error) {
    const q = `query($id:String!){ issue(id:$id){ id identifier title description url state{ name } } }`
	var resp struct {
		Issue *struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			Description string `json:"description"`
			URL        string `json:"url"`
			State      struct{ Name string `json:"name"` } `json:"state"`
		} `json:"issue"`
	}
	if err := c.do(q, map[string]interface{}{"id": id}, &resp); err != nil {
		return nil, err
	}
	if resp.Issue == nil {
		return nil, nil
	}
	is := resp.Issue
    return &Issue{ID: is.ID, Identifier: is.Identifier, Title: is.Title, Description: is.Description, URL: is.URL, StateName: is.State.Name}, nil
}

func (c *Client) IssueByKey(teamID string, number int) (*Issue, error) {
    const q = `query($teamId:ID!,$number:Float!){
  issues(first:1, filter:{ and:[ { team: { id: { eq: $teamId } } }, { number: { eq: $number } } ] }){
    nodes{ id identifier title description url state{ name } }
  }
}`
    var resp struct {
        Issues struct{
            Nodes []struct{
                ID          string `json:"id"`
                Identifier  string `json:"identifier"`
                Title       string `json:"title"`
                Description string `json:"description"`
                URL         string `json:"url"`
                State       struct{ Name string `json:"name"` } `json:"state"`
            } `json:"nodes"`
        } `json:"issues"`
    }
    if err := c.do(q, map[string]interface{}{"teamId": teamID, "number": float64(number)}, &resp); err != nil { return nil, err }
    if len(resp.Issues.Nodes) == 0 { return nil, nil }
    n := resp.Issues.Nodes[0]
    return &Issue{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name}, nil
}

// (Note) Linear's schema expects number as Float in filters

func (c *Client) CreateIssue(teamID, title, description string) (*Issue, error) {
    const q = `mutation($input: IssueCreateInput!){ issueCreate(input:$input){ success issue{ id identifier title description url state{ name } } } }`
	vars := map[string]interface{}{
		"input": map[string]interface{}{
			"teamId":      teamID,
			"title":       title,
			"description": description,
		},
	}
	var resp struct {
		IssueCreate struct {
			Success bool `json:"success"`
			Issue   *struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Title      string `json:"title"`
				Description string `json:"description"`
				URL        string `json:"url"`
				State      struct{ Name string `json:"name"` } `json:"state"`
			} `json:"issue"`
		} `json:"issueCreate"`
	}
	if err := c.do(q, vars, &resp); err != nil {
		return nil, err
	}
	if !resp.IssueCreate.Success || resp.IssueCreate.Issue == nil {
		return nil, errors.New("issue creation failed")
	}
	is := resp.IssueCreate.Issue
    return &Issue{ID: is.ID, Identifier: is.Identifier, Title: is.Title, Description: is.Description, URL: is.URL, StateName: is.State.Name}, nil
}

// --- Additional types and richer API for advanced commands ---

type Project struct {
    ID     string `json:"id"`
    Name   string `json:"name"`
    State  string `json:"state"`
    TeamID string `json:"teamId"`
    URL    string `json:"url"`
}

type User struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

type Label struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

// IssueTemplate represents a team-scoped template in Linear (if supported by schema)
type IssueTemplate struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

// ListProjects returns up to 50 accessible projects
func (c *Client) ListProjects() ([]Project, error) {
    // Minimal fields to reduce required permissions. Linear often caps page size at 50.
    const q = `query { projects(first: 50) { nodes { id name } } }`
    var resp struct {
        Projects struct {
            Nodes []struct {
                ID    string `json:"id"`
                Name  string `json:"name"`
            } `json:"nodes"`
        } `json:"projects"`
    }
    if err := c.do(q, nil, &resp); err != nil { return nil, err }
    out := make([]Project, 0, len(resp.Projects.Nodes))
    for _, n := range resp.Projects.Nodes { out = append(out, Project{ID: n.ID, Name: n.Name}) }
    return out, nil
}

// ListProjectsAll returns a larger set of projects (up to 200) for selection
func (c *Client) ListProjectsAll(limit int) ([]Project, error) {
    if limit <= 0 { limit = 200 }
    const q = `query($first:Int!){ projects(first:$first){ nodes{ id name state url } } }`
    var resp struct { Projects struct{ Nodes []struct{ ID, Name, State, URL string } `json:"nodes"` } `json:"projects"` }
    if err := c.do(q, map[string]interface{}{"first": limit}, &resp); err != nil { return nil, err }
    out := make([]Project, 0, len(resp.Projects.Nodes))
    for _, n := range resp.Projects.Nodes { out = append(out, Project{ID: n.ID, Name: n.Name, State: n.State, URL: n.URL}) }
    return out, nil
}

// ListProjectsByTeam returns projects that belong to a given team
func (c *Client) ListProjectsByTeam(teamID string, limit int) ([]Project, error) {
    if limit <= 0 { limit = 200 }
    // 1) Prefer team.projects relation when available
    {
        const q = `query($id:String!,$first:Int!){ team(id:$id){ projects(first:$first){ nodes{ id name state url } } } }`
        var resp struct {
            Team *struct{
                Projects struct{
                    Nodes []struct{ ID, Name, State, URL string } `json:"nodes"`
                } `json:"projects"`
            } `json:"team"`
        }
        if err := c.do(q, map[string]interface{}{"id": teamID, "first": limit}, &resp); err == nil && resp.Team != nil {
            nodes := resp.Team.Projects.Nodes
            if len(nodes) > 0 {
                out := make([]Project, 0, len(nodes))
                for _, n := range nodes { out = append(out, Project{ID: n.ID, Name: n.Name, State: n.State, URL: n.URL, TeamID: teamID}) }
                return out, nil
            }
        }
    }
    // 2) Try root projects filter using ID type
    {
        const q = `query($teamId:ID!,$first:Int!){ projects(first:$first, filter:{ teams:{ some:{ id:{ eq:$teamId }}}}){ nodes{ id name state url } } }`
        var resp struct { Projects struct{ Nodes []struct{ ID, Name, State, URL string } `json:"nodes"` } `json:"projects"` }
        if err := c.do(q, map[string]interface{}{"teamId": teamID, "first": limit}, &resp); err == nil && len(resp.Projects.Nodes) > 0 {
            out := make([]Project, 0, len(resp.Projects.Nodes))
            for _, n := range resp.Projects.Nodes { out = append(out, Project{ID: n.ID, Name: n.Name, State: n.State, URL: n.URL, TeamID: teamID}) }
            return out, nil
        }
    }
    // 3) Fallback to root projects filter using String type (legacy schema)
    {
        const q = `query($teamId:String!,$first:Int!){ projects(first:$first, filter:{ teams:{ some:{ id:{ eq:$teamId }}}}){ nodes{ id name state url } } }`
        var resp struct { Projects struct{ Nodes []struct{ ID, Name, State, URL string } `json:"nodes"` } `json:"projects"` }
        if err := c.do(q, map[string]interface{}{"teamId": teamID, "first": limit}, &resp); err != nil { return nil, err }
        out := make([]Project, 0, len(resp.Projects.Nodes))
        for _, n := range resp.Projects.Nodes { out = append(out, Project{ID: n.ID, Name: n.Name, State: n.State, URL: n.URL, TeamID: teamID}) }
        return out, nil
    }
}
// ListProjectsDetailed returns id, name, state, url
func (c *Client) ListProjectsDetailed() ([]Project, error) {
    const q = `query { projects(first: 50) { nodes { id name state url } } }`
    var resp struct {
        Projects struct {
            Nodes []struct {
                ID    string `json:"id"`
                Name  string `json:"name"`
                State string `json:"state"`
                URL   string `json:"url"`
            } `json:"nodes"`
        } `json:"projects"`
    }
    if err := c.do(q, nil, &resp); err != nil { return nil, err }
    out := make([]Project, 0, len(resp.Projects.Nodes))
    for _, n := range resp.Projects.Nodes { out = append(out, Project{ID: n.ID, Name: n.Name, State: n.State, URL: n.URL}) }
    return out, nil
}

// ResolveProject resolves by id (exact) or by name (exact, single)
func (c *Client) ResolveProject(input string) (*Project, error) {
    {
        const q = `query($id:String!){ project(id:$id){ id name state team { id } } }`
        var resp struct { Project *struct{ ID, Name, State string; Team *struct{ ID string } `json:"team"` } `json:"project"` }
        if err := c.do(q, map[string]interface{}{"id": input}, &resp); err == nil && resp.Project != nil {
            p := resp.Project
            var teamID string
            if p.Team != nil { teamID = p.Team.ID }
            return &Project{ID: p.ID, Name: p.Name, State: p.State, TeamID: teamID}, nil
        }
    }
    const q = `query($name:String!){ projects(filter:{ name:{ eq:$name } }, first:2){ nodes{ id name state team { id } } } }`
    var resp struct { Projects struct{ Nodes []struct{ ID, Name, State string; Team *struct{ ID string } `json:"team"` } `json:"nodes"` } `json:"projects"` }
    if err := c.do(q, map[string]interface{}{"name": input}, &resp); err != nil { return nil, err }
    if len(resp.Projects.Nodes) == 0 { return nil, nil }
    if len(resp.Projects.Nodes) > 1 { return nil, fmt.Errorf("multiple projects named '%s'", input) }
    n := resp.Projects.Nodes[0]
    var teamID string
    if n.Team != nil { teamID = n.Team.ID }
    return &Project{ID: n.ID, Name: n.Name, State: n.State, TeamID: teamID}, nil
}

// ResolveUser resolves a user by id, or by name/email (single match)
func (c *Client) ResolveUser(input string) (*User, error) {
    {
        const q = `query($id:String!){ user(id:$id){ id name email } }`
        var resp struct { User *User `json:"user"` }
        if err := c.do(q, map[string]interface{}{"id": input}, &resp); err == nil && resp.User != nil { return resp.User, nil }
    }
    const q = `query($q:String!){ users(filter:{ or:[{ name:{ contains:$q } }, { email:{ eq:$q } }] }, first:5){ nodes{ id name email } } }`
    var resp struct { Users struct{ Nodes []User `json:"nodes"` } `json:"users"` }
    if err := c.do(q, map[string]interface{}{"q": input}, &resp); err != nil { return nil, err }
    if len(resp.Users.Nodes) == 0 { return nil, nil }
    if len(resp.Users.Nodes) > 1 { return nil, fmt.Errorf("multiple users match '%s'", input) }
    u := resp.Users.Nodes[0]
    return &u, nil
}

// ResolveLabelByName resolves a label by exact name
func (c *Client) ResolveLabelByName(name string) (*Label, error) {
    const q = `query($name:String!){ issueLabels(filter:{ name:{ eq:$name } }, first:2){ nodes{ id name } } }`
    var resp struct { IssueLabels struct{ Nodes []Label `json:"nodes"` } `json:"issueLabels"` }
    if err := c.do(q, map[string]interface{}{"name": name}, &resp); err != nil { return nil, err }
    if len(resp.IssueLabels.Nodes) == 0 { return nil, nil }
    if len(resp.IssueLabels.Nodes) > 1 { return nil, fmt.Errorf("multiple labels named '%s'", name) }
    l := resp.IssueLabels.Nodes[0]
    return &l, nil
}

// ListIssueLabels returns up to 200 labels accessible to the token
func (c *Client) ListIssueLabels(limit int) ([]Label, error) {
    if limit <= 0 { limit = 200 }
    const q = `query($first:Int!){ issueLabels(first:$first){ nodes{ id name } } }`
    var resp struct { IssueLabels struct{ Nodes []Label `json:"nodes"` } `json:"issueLabels"` }
    if err := c.do(q, map[string]interface{}{"first": limit}, &resp); err != nil { return nil, err }
    return resp.IssueLabels.Nodes, nil
}

// IssueDetails is a richer issue payload for view/list
type IssueDetails struct {
    ID         string   `json:"id"`
    Identifier string   `json:"identifier"`
    Title      string   `json:"title"`
    Description string  `json:"description"`
    URL        string   `json:"url"`
    StateName  string   `json:"stateName"`
    Assignee   *User    `json:"assignee,omitempty"`
    Labels     []Label  `json:"labels"`
    Project    *Project `json:"project,omitempty"`
    Comments   []Comment `json:"comments,omitempty"`
}

// GetIssueDetails returns a full issue by id
func (c *Client) GetIssueDetails(id string) (*IssueDetails, error) {
    const q = `query($id:String!){ issue(id:$id){ id identifier title description url state{ name } assignee{ id name email } labels{ nodes{ id name } } project{ id name state } } }`
    var resp struct { Issue *struct { ID, Identifier, Title, Description, URL string; State struct{ Name string `json:"name"` } `json:"state"`; Assignee *User `json:"assignee"`; Labels struct{ Nodes []Label `json:"nodes"` } `json:"labels"`; Project *struct{ ID, Name, State string } `json:"project"` } `json:"issue"` }
    if err := c.do(q, map[string]interface{}{"id": id}, &resp); err != nil { return nil, err }
    if resp.Issue == nil { return nil, nil }
    n := resp.Issue
    var proj *Project
    if n.Project != nil { proj = &Project{ID: n.Project.ID, Name: n.Project.Name, State: n.Project.State} }
    return &IssueDetails{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name, Assignee: n.Assignee, Labels: n.Labels.Nodes, Project: proj}, nil
}

// GetIssueDetailsWithComments returns full issue details plus up to N comments
func (c *Client) GetIssueDetailsWithComments(id string, commentsLimit int) (*IssueDetails, error) {
    det, err := c.GetIssueDetails(id)
    if err != nil || det == nil { return det, err }
    if commentsLimit <= 0 { return det, nil }
    comments, err := c.IssueComments(id, commentsLimit)
    if err != nil { return nil, err }
    det.Comments = comments
    return det, nil
}

// IssueListFilter supports optional filters for listing
type IssueListFilter struct {
    ProjectID  string
    AssigneeID string
    StateName  string
    Limit      int
}

// ListIssuesFiltered returns issues matching optional filters
func (c *Client) ListIssuesFiltered(f IssueListFilter) ([]IssueDetails, error) {
    if f.Limit <= 0 { f.Limit = 10 }
    const q = `query($first:Int!,$projectId:ID,$assigneeId:ID,$state:String){
issues(first:$first, filter:{ and:[ { project: { id: { eq: $projectId } } }, { assignee: { id: { eq: $assigneeId } } }, { state: { name: { eq: $state } } } ] }){
  nodes{ id identifier title url state{ name } assignee{ id name email } labels{ nodes{ id name } } project{ id name state } }
}}
`
    vars := map[string]interface{}{"first": f.Limit}
    if f.ProjectID != "" { vars["projectId"] = f.ProjectID }
    if f.AssigneeID != "" { vars["assigneeId"] = f.AssigneeID }
    if f.StateName != "" { vars["state"] = f.StateName }
    var resp struct { Issues struct{ Nodes []struct { ID, Identifier, Title, URL string; State struct{ Name string `json:"name"` } `json:"state"`; Assignee *User `json:"assignee"`; Labels struct{ Nodes []Label `json:"nodes"` } `json:"labels"`; Project *struct{ ID, Name, State string } `json:"project"` } `json:"nodes"` } `json:"issues"` }
    if err := c.do(q, vars, &resp); err != nil { return nil, err }
    out := make([]IssueDetails, 0, len(resp.Issues.Nodes))
    for _, n := range resp.Issues.Nodes {
        var proj *Project
        if n.Project != nil { proj = &Project{ID: n.Project.ID, Name: n.Project.Name, State: n.Project.State} }
        out = append(out, IssueDetails{ID: n.ID, Identifier: n.Identifier, Title: n.Title, URL: n.URL, StateName: n.State.Name, Assignee: n.Assignee, Labels: n.Labels.Nodes, Project: proj})
    }
    return out, nil
}

// IssueCreateInput allows richer creation with project/assignee/labels/priority
type IssueCreateInput struct {
    ProjectID   string
    TeamID      string
    StateID     string
    TemplateID  string
    Title       string
    Description string
    AssigneeID  string
    LabelIDs    []string
    Priority    *int
}

// CreateIssueAdvanced creates an issue with additional fields
func (c *Client) CreateIssueAdvanced(in IssueCreateInput) (*IssueDetails, error) {
    input := map[string]interface{}{
        "title":       in.Title,
        "description": in.Description,
    }
    if in.ProjectID != "" { input["projectId"] = in.ProjectID }
    if in.TeamID != "" { input["teamId"] = in.TeamID }
    if in.StateID != "" { input["stateId"] = in.StateID }
    if in.TemplateID != "" { input["templateId"] = in.TemplateID }
    if in.AssigneeID != "" { input["assigneeId"] = in.AssigneeID }
    if len(in.LabelIDs) > 0 { input["labelIds"] = in.LabelIDs }
    if in.Priority != nil { input["priority"] = *in.Priority }

    const q = `mutation($input: IssueCreateInput!){ issueCreate(input:$input){ success issue{ id identifier title description url state{ name } assignee{ id name email } labels{ nodes{ id name } } project{ id name state } } } }`
    var resp struct { IssueCreate struct{ Success bool `json:"success"`; Issue *struct { ID, Identifier, Title, Description, URL string; State struct{ Name string `json:"name"` } `json:"state"`; Assignee *User `json:"assignee"`; Labels struct{ Nodes []Label `json:"nodes"` } `json:"labels"`; Project *struct{ ID, Name, State string } `json:"project"` } `json:"issue"` } `json:"issueCreate"` }
    if err := c.do(q, map[string]interface{}{"input": input}, &resp); err != nil { return nil, err }
    if !resp.IssueCreate.Success || resp.IssueCreate.Issue == nil { return nil, errors.New("issue creation failed") }
    n := resp.IssueCreate.Issue
    var proj *Project
    if n.Project != nil { proj = &Project{ID: n.Project.ID, Name: n.Project.Name, State: n.Project.State} }
    return &IssueDetails{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name, Assignee: n.Assignee, Labels: n.Labels.Nodes, Project: proj}, nil
}

// UpdateIssue updates an existing issue's description and/or title
func (c *Client) UpdateIssue(issueID, title, description string) (*IssueDetails, error) {
    if issueID == "" {
        return nil, errors.New("issueID cannot be empty")
    }
    
    input := map[string]interface{}{"id": issueID}
    if title != "" { input["title"] = title }
    if description != "" { input["description"] = description }

    const q = `mutation($input: IssueUpdateInput!){ issueUpdate(input:$input){ success issue{ id identifier title description url state{ name } assignee{ id name email } labels{ nodes{ id name } } project{ id name state } } } }`
    var resp struct { IssueUpdate struct{ Success bool `json:"success"`; Issue *struct { ID, Identifier, Title, Description, URL string; State struct{ Name string `json:"name"` } `json:"state"`; Assignee *User `json:"assignee"`; Labels struct{ Nodes []Label `json:"nodes"` } `json:"labels"`; Project *struct{ ID, Name, State string } `json:"project"` } `json:"issue"` } `json:"issueUpdate"` }
    if err := c.do(q, map[string]interface{}{"input": input}, &resp); err != nil { return nil, err }
    if !resp.IssueUpdate.Success || resp.IssueUpdate.Issue == nil { return nil, errors.New("issue update failed") }
    n := resp.IssueUpdate.Issue
    var proj *Project
    if n.Project != nil { proj = &Project{ID: n.Project.ID, Name: n.Project.Name, State: n.Project.State} }
    return &IssueDetails{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name, Assignee: n.Assignee, Labels: n.Labels.Nodes, Project: proj}, nil
}

// State represents a workflow state in a team
type State struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Type     string `json:"type"`
    Position int    `json:"position"`
}

// TeamStates lists the workflow states for a given team
func (c *Client) TeamStates(teamID string) ([]State, error) {
    const q = `query($id:String!){ team(id:$id){ states(first:100){ nodes{ id name type position } } } }`
    var resp struct{ Team *struct{ States struct{ Nodes []State `json:"nodes"` } `json:"states"` } `json:"team"` }
    if err := c.do(q, map[string]interface{}{"id": teamID}, &resp); err != nil { return nil, err }
    if resp.Team == nil { return nil, nil }
    return resp.Team.States.Nodes, nil
}

// TeamMembers lists users who are members of the given team
func (c *Client) TeamMembers(teamID string) ([]User, error) {
    const q = `query($id:String!){ team(id:$id){ members(first:200){ nodes{ user{ id name email } } } } }`
    var resp struct{ Team *struct{ Members struct{ Nodes []struct{ User User `json:"user"` } `json:"nodes"` } `json:"members"` } `json:"team"` }
    if err := c.do(q, map[string]interface{}{"id": teamID}, &resp); err != nil { return nil, err }
    if resp.Team == nil { return nil, nil }
    users := make([]User, 0, len(resp.Team.Members.Nodes))
    for _, n := range resp.Team.Members.Nodes { users = append(users, n.User) }
    return users, nil
}

// ListIssueTemplatesForTeam tries to query templates via Team.issueTemplates, falling back to root issueTemplates with team filter.
func (c *Client) ListIssueTemplatesForTeam(teamID string) ([]IssueTemplate, error) {
    // Attempt Team.issueTemplates
    {
        const q = `query($teamId:String!){ team(id:$teamId){ issueTemplates(first:100){ nodes{ id name description } } } }`
        var resp struct{ Team *struct{ IssueTemplates *struct{ Nodes []IssueTemplate `json:"nodes"` } `json:"issueTemplates"` } `json:"team"` }
        if err := c.do(q, map[string]interface{}{"teamId": teamID}, &resp); err == nil && resp.Team != nil && resp.Team.IssueTemplates != nil {
            return resp.Team.IssueTemplates.Nodes, nil
        }
    }
    // Alternative Team.templates
    {
        const q = `query($teamId:String!){ team(id:$teamId){ templates(first:100){ nodes{ id name description } } } }`
        var resp struct{ Team *struct{ Templates *struct{ Nodes []IssueTemplate `json:"nodes"` } `json:"templates"` } `json:"team"` }
        if err := c.do(q, map[string]interface{}{"teamId": teamID}, &resp); err == nil && resp.Team != nil && resp.Team.Templates != nil {
            return resp.Team.Templates.Nodes, nil
        }
    }
    // Fallback root connection with filter
    const q = `query($teamId:String!){ issueTemplates(first:100, filter:{ team:{ id:{ eq:$teamId }}}){ nodes{ id name description } } }`
    var resp struct{ IssueTemplates struct{ Nodes []IssueTemplate `json:"nodes"` } `json:"issueTemplates"` }
    if err := c.do(q, map[string]interface{}{"teamId": teamID}, &resp); err == nil && len(resp.IssueTemplates.Nodes) > 0 {
        return resp.IssueTemplates.Nodes, nil
    }
    // Alternative root templates
    {
        const q2 = `query($teamId:String!){ templates(first:100, filter:{ team:{ id:{ eq:$teamId }}}){ nodes{ id name description } } }`
        var resp2 struct{ Templates struct{ Nodes []IssueTemplate `json:"nodes"` } `json:"templates"` }
        if err := c.do(q2, map[string]interface{}{"teamId": teamID}, &resp2); err == nil {
            return resp2.Templates.Nodes, nil
        }
    }
    return nil, nil
}

// IssueTemplateByID fetches a single template by id using issueTemplate field, falling back to filtering connection
func (c *Client) IssueTemplateByID(id string) (*IssueTemplate, error) {
    {
        const q = `query($id:String!){ issueTemplate(id:$id){ id name description } }`
        var resp struct{ IssueTemplate *IssueTemplate `json:"issueTemplate"` }
        if err := c.do(q, map[string]interface{}{"id": id}, &resp); err == nil && resp.IssueTemplate != nil {
            return resp.IssueTemplate, nil
        }
    }
    {
        const q = `query($id:String!){ template(id:$id){ id name description } }`
        var resp struct{ Template *IssueTemplate `json:"template"` }
        if err := c.do(q, map[string]interface{}{"id": id}, &resp); err == nil && resp.Template != nil {
            return resp.Template, nil
        }
    }
    const q = `query($id:String!){ issueTemplates(first:1, filter:{ id:{ eq:$id }}){ nodes{ id name description } } }`
    var resp struct{ IssueTemplates struct{ Nodes []IssueTemplate `json:"nodes"` } `json:"issueTemplates"` }
    if err := c.do(q, map[string]interface{}{"id": id}, &resp); err == nil && len(resp.IssueTemplates.Nodes) > 0 {
        t := resp.IssueTemplates.Nodes[0]
        return &t, nil
    }
    {
        const q2 = `query($id:String!){ templates(first:1, filter:{ id:{ eq:$id }}){ nodes{ id name description } } }`
        var resp2 struct{ Templates struct{ Nodes []IssueTemplate `json:"nodes"` } `json:"templates"` }
        if err := c.do(q2, map[string]interface{}{"id": id}, &resp2); err == nil && len(resp2.Templates.Nodes) > 0 {
            t := resp2.Templates.Nodes[0]
            return &t, nil
        }
    }
    return nil, nil
}

// IssueTemplateByNameForTeam resolves a template by exact name within a team
func (c *Client) IssueTemplateByNameForTeam(teamID, name string) (*IssueTemplate, error) {
    // Use the same successful query path as ListIssueTemplatesForTeam, then filter client-side
    templates, err := c.ListIssueTemplatesForTeam(teamID)
    if err != nil {
        return nil, err
    }
    
    // Exact match first
    for _, t := range templates {
        if strings.TrimSpace(t.Name) == strings.TrimSpace(name) {
            return &t, nil
        }
    }
    
    // Case-insensitive match
    targetLower := strings.ToLower(strings.TrimSpace(name))
    for _, t := range templates {
        if strings.ToLower(strings.TrimSpace(t.Name)) == targetLower {
            return &t, nil
        }
    }
    
    return nil, nil
}

// TemplateBodyByIDDynamic attempts to read a template's body using the best-available field.
// It introspects the Template type fields and prefers these in order: content, body, description, markdown, text.
func (c *Client) TemplateBodyByIDDynamic(id string) (title string, body string, usedField string, err error) {
    // Introspect Template fields
    fields, _ := c.TemplateTypeFieldNames()
    toLowerSet := func(ss []string) map[string]struct{} {
        m := map[string]struct{}{}
        for _, s := range ss { m[strings.ToLower(strings.TrimSpace(s))] = struct{}{} }
        return m
    }
    present := toLowerSet(fields)
    // Candidate scalar fields in priority order
    scalar := []string{"content", "contentmarkdown", "contentmd", "body", "bodymarkdown", "markdown", "text", "description"}
    // Build selection with whichever scalar fields are present
    sels := []string{"name"}
    picks := []string{}
    for _, f := range scalar {
        if _, ok := present[f]; ok { sels = append(sels, f); picks = append(picks, f) }
    }
    hasBlocks := false
    if _, ok := present["blocks"]; ok { hasBlocks = true; sels = append(sels, "blocks { markdown content text body description }") }
    q := "query($id:String!){ template(id:$id){ " + strings.Join(sels, " ") + " } }"
    var out map[string]any
    if e := c.do(q, map[string]any{"id": id}, &out); e != nil { return "", "", "", e }
    tpl, _ := out["template"].(map[string]any)
    if tpl == nil { return "", "", "", nil }
    // title
    if n, ok := tpl["name"].(string); ok { title = n }
    // try scalar picks in order
    for _, key := range picks {
        if v, ok := tpl[key]; ok {
            if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" { return title, s, key, nil }
        }
    }
    // try blocks aggregation
    if hasBlocks {
        if arr, ok := tpl["blocks"].([]any); ok && len(arr) > 0 {
            var b strings.Builder
            for i, it := range arr {
                m, _ := it.(map[string]any)
                if m == nil { continue }
                // choose best available subfield
                for _, sub := range []string{"markdown", "content", "text", "body", "description"} {
                    if v, ok := m[sub]; ok {
                        if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
                            if i > 0 { b.WriteString("\n\n") }
                            b.WriteString(s)
                            break
                        }
                    }
                }
            }
            if bs := strings.TrimSpace(b.String()); bs != "" { return title, bs, "blocks", nil }
        }
    }
    // Fallback to description if present even empty
    if v, ok := tpl["description"]; ok {
        if s, ok2 := v.(string); ok2 { return title, s, "description", nil }
    }
    return title, "", "", nil
}

// IssueTemplateByNameForTeamFull finds a template by (case-insensitive) name within a team
// and returns its title and body using dynamic field detection.
func (c *Client) IssueTemplateByNameForTeamFull(teamID, name string) (title string, body string, err error) {
    items, e := c.ListIssueTemplatesForTeam(teamID)
    if e != nil { return "", "", e }
    if len(items) == 0 { return "", "", nil }
    target := strings.ToLower(strings.TrimSpace(name))
    var picked *IssueTemplate
    for _, it := range items { if strings.ToLower(strings.TrimSpace(it.Name)) == target { picked = &it; break } }
    if picked == nil {
        for _, it := range items { if strings.Contains(strings.ToLower(strings.TrimSpace(it.Name)), target) { picked = &it; break } }
    }
    if picked == nil { return "", "", nil }
    t, b, _, e2 := c.TemplateBodyByIDDynamic(picked.ID)
    if e2 != nil { return "", "", e2 }
    return t, b, nil
}

// TemplateTypeFieldNames returns all field names on the Template GraphQL type
func (c *Client) TemplateTypeFieldNames() ([]string, error) {
    const q = `query{ __type(name:"Template"){ fields{ name } } }`
    var resp struct{ Type *struct{ Fields []struct{ Name string `json:"name"` } `json:"fields"` } `json:"__type"` }
    if err := c.do(q, nil, &resp); err != nil { return nil, err }
    if resp.Type == nil { return nil, nil }
    out := make([]string, 0, len(resp.Type.Fields))
    for _, f := range resp.Type.Fields { out = append(out, f.Name) }
    return out, nil
}

// TemplateNodeByIDRaw returns a map of selected fields for a template node, using a safe intersection
// of common field names and fields present in the schema.
func (c *Client) TemplateNodeByIDRaw(id string) (map[string]any, error) {
    fields, _ := c.TemplateTypeFieldNames()
    allowed := map[string]struct{}{ "id":{}, "name":{}, "content":{}, "body":{}, "description":{}, "markdown":{}, "text":{} }
    sels := []string{"id", "name"}
    for _, f := range fields {
        lf := strings.ToLower(strings.TrimSpace(f))
        if _, ok := allowed[lf]; ok && lf != "id" && lf != "name" { sels = append(sels, lf) }
    }
    if len(sels) == 2 { // fallback minimal
        sels = append(sels, "description")
    }
    sel := strings.Join(sels, " ")
    q := "query($id:String!){ template(id:$id){ " + sel + " } }"
    var out map[string]any
    if err := c.do(q, map[string]any{"id": id}, &out); err != nil { return nil, err }
    node, _ := out["template"].(map[string]any)
    return node, nil
}

// FindTemplateForTeamByKeywords lists team templates and returns the first whose name
// matches any of the provided keywords using loose, case-insensitive matching.
// It strips common suffixes like "template" for better matching.
func (c *Client) FindTemplateForTeamByKeywords(teamID string, keywords []string) (*IssueTemplate, error) {
    items, err := c.ListIssueTemplatesForTeam(teamID)
    if err != nil { return nil, err }
    if len(items) == 0 { return nil, nil }
    normalize := func(s string) string {
        s = strings.ToLower(strings.TrimSpace(s))
        s = strings.TrimSuffix(s, " template")
        s = strings.TrimSuffix(s, " templates")
        return strings.TrimSpace(s)
    }
    for _, kw := range keywords {
        k := normalize(kw)
        // exact match first
        for _, it := range items { if normalize(it.Name) == k { return &it, nil } }
        // contains match as fallback
        for _, it := range items { if strings.Contains(normalize(it.Name), k) { return &it, nil } }
    }
    return nil, nil
}

// CreateIssueFromTemplate attempts to create an issue using templateId in IssueCreateInput
func (c *Client) CreateIssueFromTemplate(teamID, templateID, title string) (*IssueDetails, error) {
    // Backwards-compatible convenience wrapper
    return c.CreateIssueAdvanced(IssueCreateInput{TeamID: teamID, TemplateID: templateID, Title: title})
}

// SupportsIssueCreateTemplateId checks if IssueCreateInput has templateId
func (c *Client) SupportsIssueCreateTemplateId() bool {
    // Reuse supportsTemplates cache if already checked; otherwise introspect input fields
    const q = `query{ __type(name:"IssueCreateInput"){ inputFields{ name } } }`
    var resp struct{ Type *struct{ InputFields []struct{ Name string `json:"name"` } `json:"inputFields"` } `json:"__type"` }
    if err := c.do(q, nil, &resp); err != nil || resp.Type == nil { return false }
    for _, f := range resp.Type.InputFields { if strings.EqualFold(f.Name, "templateId") { return true } }
    return false
}

// --- Comments ---

type Comment struct {
    ID   string `json:"id"`
    Body string `json:"body"`
}

type CommentResult struct {
    Comment   Comment `json:"comment"`
    IssueID   string  `json:"issueId"`
    IssueURL  string  `json:"issueUrl"`
    IssueKey  string  `json:"issueKey"`
}

func (c *Client) CreateComment(issueID, body string) (*CommentResult, error) {
    const q = `mutation($input: CommentCreateInput!){ commentCreate(input:$input){ success comment{ id body issue{ id url identifier } } } }`
    vars := map[string]interface{}{
        "input": map[string]interface{}{
            "issueId": issueID,
            "body":    body,
        },
    }
    var resp struct {
        CommentCreate struct {
            Success bool `json:"success"`
            Comment *struct {
                ID    string `json:"id"`
                Body  string `json:"body"`
                Issue struct{
                    ID         string `json:"id"`
                    URL        string `json:"url"`
                    Identifier string `json:"identifier"`
                } `json:"issue"`
            } `json:"comment"`
        } `json:"commentCreate"`
    }
    if err := c.do(q, vars, &resp); err != nil { return nil, err }
    if !resp.CommentCreate.Success || resp.CommentCreate.Comment == nil { return nil, errors.New("comment creation failed") }
    n := resp.CommentCreate.Comment
    return &CommentResult{Comment: Comment{ID: n.ID, Body: n.Body}, IssueID: n.Issue.ID, IssueURL: n.Issue.URL, IssueKey: n.Issue.Identifier}, nil
}

// IssueComments fetches up to limit comments for an issue (minimal fields for compatibility)
func (c *Client) IssueComments(issueID string, limit int) ([]Comment, error) {
    if limit <= 0 { limit = 20 }
    const q = `query($id:String!,$first:Int!){ issue(id:$id){ comments(first:$first){ nodes{ id body } } } }`
    var resp struct {
        Issue *struct {
            Comments struct{
                Nodes []Comment `json:"nodes"`
            } `json:"comments"`
        } `json:"issue"`
    }
    if err := c.do(q, map[string]interface{}{"id": issueID, "first": limit}, &resp); err != nil { return nil, err }
    if resp.Issue == nil { return nil, nil }
    return resp.Issue.Comments.Nodes, nil
}

// ListTeams returns all teams the user has access to
func (c *Client) ListTeams() ([]Team, error) {
    const q = `query{ teams(first:100){ nodes{ id key name } } }`
    var resp struct {
        Teams struct {
            Nodes []Team `json:"nodes"`
        } `json:"teams"`
    }
    if err := c.do(q, nil, &resp); err != nil {
        return nil, err
    }
    return resp.Teams.Nodes, nil
}


