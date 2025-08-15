package api

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "regexp"
    "testing"
)

type gqlPayload struct {
    Query     string                 `json:"query"`
    Variables map[string]interface{} `json:"variables"`
}

func newTestClient(t *testing.T, handler func(t *testing.T, w http.ResponseWriter, r *http.Request)) *Client {
    t.Helper()
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        handler(t, w, r)
    }))
    t.Cleanup(srv.Close)

    c := NewClient("test-key")
    c.endpoint = srv.URL
    // Use the server's default client without redirects etc; http.DefaultClient is fine
    return c
}

func readGQL(t *testing.T, r *http.Request) gqlPayload {
    t.Helper()
    var p gqlPayload
    if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
        t.Fatalf("failed to decode gql payload: %v", err)
    }
    return p
}

func respondJSON(w http.ResponseWriter, v any) {
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(v)
}

func TestIssueByKey_UsesIDTeamIdVarType(t *testing.T) {
    // Expect: query($teamId:ID!,$number:Float!)
    c := newTestClient(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
        p := readGQL(t, r)
        must := regexp.MustCompile(`(?s)query\s*\(\s*\$teamId:\s*ID!\s*,\s*\$number:\s*Float!\s*\)\s*{`)
        if !must.MatchString(p.Query) {
            t.Fatalf("query did not declare $teamId as ID!: %s", p.Query)
        }
        // Return minimal response
        respondJSON(w, map[string]any{
            "data": map[string]any{
                "issues": map[string]any{
                    "nodes": []any{
                        map[string]any{
                            "id": "iss_1",
                            "identifier": "POK-28",
                            "title": "Example",
                            "description": "Desc",
                            "url": "https://example",
                            "state": map[string]any{"name": "Todo"},
                        },
                    },
                },
            },
        })
    })

    got, err := c.IssueByKey("team_1", 28)
    if err != nil { t.Fatalf("IssueByKey error: %v", err) }
    if got == nil || got.ID == "" { t.Fatalf("IssueByKey got nil or empty result") }
}

func TestIssueByID_UsesStringVarType(t *testing.T) {
    c := newTestClient(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
        p := readGQL(t, r)
        must := regexp.MustCompile(`(?s)query\s*\(\s*\$id:\s*String!\s*\)\s*{\s*issue\(id:\$id\)`)
        if !must.MatchString(p.Query) {
            t.Fatalf("query did not declare $id as String!: %s", p.Query)
        }
        respondJSON(w, map[string]any{
            "data": map[string]any{
                "issue": map[string]any{
                    "id": "iss_1",
                    "identifier": "POK-28",
                    "title": "Example",
                    "description": "Desc",
                    "url": "https://example",
                    "state": map[string]any{"name": "Todo"},
                },
            },
        })
    })

    got, err := c.IssueByID("iss_1")
    if err != nil { t.Fatalf("IssueByID error: %v", err) }
    if got == nil || got.ID == "" { t.Fatalf("IssueByID got nil or empty result") }
}

func TestGetIssueDetails_UsesStringVarType(t *testing.T) {
    c := newTestClient(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
        p := readGQL(t, r)
        must := regexp.MustCompile(`(?s)query\s*\(\s*\$id:\s*String!\s*\)\s*{\s*issue\(id:\$id\)`)
        if !must.MatchString(p.Query) {
            t.Fatalf("query did not declare $id as String!: %s", p.Query)
        }
        respondJSON(w, map[string]any{
            "data": map[string]any{
                "issue": map[string]any{
                    "id": "iss_1",
                    "identifier": "POK-28",
                    "title": "Example",
                    "description": "Desc",
                    "url": "https://example",
                    "state": map[string]any{"name": "Todo"},
                    "assignee": nil,
                    "labels": map[string]any{"nodes": []any{}},
                    "project": nil,
                },
            },
        })
    })

    got, err := c.GetIssueDetails("iss_1")
    if err != nil { t.Fatalf("GetIssueDetails error: %v", err) }
    if got == nil || got.ID == "" { t.Fatalf("GetIssueDetails got nil or empty result") }
}

func TestIssueComments_UsesStringVarType(t *testing.T) {
    c := newTestClient(t, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
        p := readGQL(t, r)
        must := regexp.MustCompile(`(?s)query\s*\(\s*\$id:\s*String!\s*,\s*\$first:\s*Int!\s*\)\s*{\s*issue\(id:\$id\)`)
        if !must.MatchString(p.Query) {
            t.Fatalf("query did not declare $id as String! and $first as Int!: %s", p.Query)
        }
        respondJSON(w, map[string]any{
            "data": map[string]any{
                "issue": map[string]any{
                    "comments": map[string]any{
                        "nodes": []any{
                            map[string]any{"id": "c1", "body": "hi"},
                        },
                    },
                },
            },
        })
    })

    got, err := c.IssueComments("iss_1", 1)
    if err != nil { t.Fatalf("IssueComments error: %v", err) }
    if len(got) != 1 || got[0].ID != "c1" { t.Fatalf("IssueComments unexpected result: %+v", got) }
}
