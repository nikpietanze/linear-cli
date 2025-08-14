package api

import (
    "bytes"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "regexp"
    "strings"
    "time"
)

type Client struct {
	httpClient *http.Client
	apiKey     string
	endpoint   string
    allowedMutations map[string]struct{}
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
	TeamKey     string `json:"teamKey"`
	URL         string `json:"url"`
}

func NewClient(apiKey string) *Client {
    return &Client{
        httpClient: &http.Client{Timeout: 15 * time.Second},
        apiKey:     apiKey,
        endpoint:   "https://api.linear.app/graphql",
        allowedMutations: map[string]struct{}{
            "issueCreate": {},
            "commentCreate": {},
        },
    }
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
        if idx := strings.Index(s, ":"); idx >= 0 { s = strings.TrimSpace(s[idx+1:]) }
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
		const q = `query($first:Int!){ issues(first:$first, orderBy: updatedAt){ nodes{ id identifier title description url state{ name } team{ key } } } }`
		var resp struct {
			Issues struct {
				Nodes []struct {
					ID         string `json:"id"`
					Identifier string `json:"identifier"`
					Title      string `json:"title"`
					Description string `json:"description"`
					URL        string `json:"url"`
					State      struct{ Name string `json:"name"` } `json:"state"`
					Team       struct{ Key string `json:"key"` } `json:"team"`
				} `json:"nodes"`
			} `json:"issues"`
		}
		if err := c.do(q, map[string]interface{}{"first": limit}, &resp); err != nil {
			return nil, err
		}
		issues := make([]Issue, 0, len(resp.Issues.Nodes))
		for _, n := range resp.Issues.Nodes {
			issues = append(issues, Issue{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name, TeamKey: n.Team.Key})
		}
		return issues, nil
	}
	const q = `query($first:Int!,$teamId:String!){ issues(first:$first, filter:{ team: { id: { eq:$teamId } } }, orderBy: updatedAt){ nodes{ id identifier title description url state{ name } team{ key } } } }`
	var resp struct {
		Issues struct {
			Nodes []struct {
				ID         string `json:"id"`
				Identifier string `json:"identifier"`
				Title      string `json:"title"`
				Description string `json:"description"`
				URL        string `json:"url"`
				State      struct{ Name string `json:"name"` } `json:"state"`
				Team       struct{ Key string `json:"key"` } `json:"team"`
			} `json:"nodes"`
		} `json:"issues"`
	}
	if err := c.do(q, map[string]interface{}{"first": limit, "teamId": teamID}, &resp); err != nil {
		return nil, err
	}
	issues := make([]Issue, 0, len(resp.Issues.Nodes))
	for _, n := range resp.Issues.Nodes {
		issues = append(issues, Issue{ID: n.ID, Identifier: n.Identifier, Title: n.Title, Description: n.Description, URL: n.URL, StateName: n.State.Name, TeamKey: n.Team.Key})
	}
	return issues, nil
}

func (c *Client) IssueByID(id string) (*Issue, error) {
	const q = `query($id:String!){ issue(id:$id){ id identifier title description url state{ name } team{ key } } }`
	var resp struct {
		Issue *struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			Description string `json:"description"`
			URL        string `json:"url"`
			State      struct{ Name string `json:"name"` } `json:"state"`
			Team       struct{ Key string `json:"key"` } `json:"team"`
		} `json:"issue"`
	}
	if err := c.do(q, map[string]interface{}{"id": id}, &resp); err != nil {
		return nil, err
	}
	if resp.Issue == nil {
		return nil, nil
	}
	is := resp.Issue
	return &Issue{ID: is.ID, Identifier: is.Identifier, Title: is.Title, Description: is.Description, URL: is.URL, StateName: is.State.Name, TeamKey: is.Team.Key}, nil
}

func (c *Client) IssueByKey(teamID string, number int) (*Issue, error) {
	const q = `query($teamId:String!,$identifier:Int!){ issueByIdentifier(teamId:$teamId, identifier:$identifier){ id identifier title description url state{ name } team{ key } } }`
	var resp struct {
		IssueByIdentifier *struct {
			ID         string `json:"id"`
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			Description string `json:"description"`
			URL        string `json:"url"`
			State      struct{ Name string `json:"name"` } `json:"state"`
			Team       struct{ Key string `json:"key"` } `json:"team"`
		} `json:"issueByIdentifier"`
	}
	if err := c.do(q, map[string]interface{}{"teamId": teamID, "identifier": number}, &resp); err != nil {
		return nil, err
	}
	if resp.IssueByIdentifier == nil {
		return nil, nil
	}
	is := resp.IssueByIdentifier
	return &Issue{ID: is.ID, Identifier: is.Identifier, Title: is.Title, Description: is.Description, URL: is.URL, StateName: is.State.Name, TeamKey: is.Team.Key}, nil
}

func (c *Client) CreateIssue(teamID, title, description string) (*Issue, error) {
	const q = `mutation($input: IssueCreateInput!){ issueCreate(input:$input){ success issue{ id identifier title description url state{ name } team{ key } } } }`
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
				Team       struct{ Key string `json:"key"` } `json:"team"`
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
	return &Issue{ID: is.ID, Identifier: is.Identifier, Title: is.Title, Description: is.Description, URL: is.URL, StateName: is.State.Name, TeamKey: is.Team.Key}, nil
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
        const q = `query($id:ID!){ project(id:$id){ id name state } }`
        var resp struct { Project *struct{ ID, Name, State string } `json:"project"` }
        if err := c.do(q, map[string]interface{}{"id": input}, &resp); err == nil && resp.Project != nil {
            p := resp.Project
            return &Project{ID: p.ID, Name: p.Name, State: p.State}, nil
        }
    }
    const q = `query($name:String!){ projects(filter:{ name:{ eq:$name } }, first:2){ nodes{ id name state } } }`
    var resp struct { Projects struct{ Nodes []struct{ ID, Name, State string } `json:"nodes"` } `json:"projects"` }
    if err := c.do(q, map[string]interface{}{"name": input}, &resp); err != nil { return nil, err }
    if len(resp.Projects.Nodes) == 0 { return nil, nil }
    if len(resp.Projects.Nodes) > 1 { return nil, fmt.Errorf("multiple projects named '%s'", input) }
    n := resp.Projects.Nodes[0]
    return &Project{ID: n.ID, Name: n.Name, State: n.State}, nil
}

// ResolveUser resolves a user by id, or by name/email (single match)
func (c *Client) ResolveUser(input string) (*User, error) {
    {
    const q = `query($id:ID!){ user(id:$id){ id name email } }`
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
    const q = `query($id:ID!){ issue(id:$id){ id identifier title description url state{ name } assignee{ id name email } labels{ nodes{ id name } } project{ id name state } } }`
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
    const q = `query($first:Int!,$projectId:String,$assigneeId:String,$state:String){
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
    ProjectOrTeamID string
    Title           string
    Description     string
    AssigneeID      string
    LabelIDs        []string
    Priority        *int
}

// CreateIssueAdvanced creates an issue with additional fields
func (c *Client) CreateIssueAdvanced(in IssueCreateInput) (*IssueDetails, error) {
    input := map[string]interface{}{
        "title":       in.Title,
        "description": in.Description,
    }
    if in.ProjectOrTeamID != "" { input["projectId"] = in.ProjectOrTeamID }
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
    const q = `query($id:ID!,$first:Int!){ issue(id:$id){ comments(first:$first){ nodes{ id body } } } }`
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
