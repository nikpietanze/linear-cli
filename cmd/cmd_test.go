package cmd

import (
    "io"
    "net/http"
    "net/http/httptest"
    "os"
    "regexp"
    "strings"
    "testing"
)

// helper to run a command and capture stdout/stderr
func runCLI(t *testing.T, args ...string) (stdout string, stderr string, exitErr error) {
    t.Helper()
    // Temporarily replace os.Args and capture output by redirecting
    oldArgs := os.Args
    os.Args = append([]string{"linear-cli"}, args...)
    t.Cleanup(func(){ os.Args = oldArgs })

    // Capture stdio
    oldOut, oldErr := os.Stdout, os.Stderr
    rOut, wOut, _ := os.Pipe()
    rErr, wErr, _ := os.Pipe()
    os.Stdout, os.Stderr = wOut, wErr
    t.Cleanup(func(){ os.Stdout, os.Stderr = oldOut, oldErr })

    // Run
    var runErr error
    func(){
        defer func(){ if r := recover(); r != nil { runErr = panicErr(r) } }()
        Execute()
    }()

    // Close writers then read all
    _ = wOut.Close(); _ = wErr.Close()
    outBytes, _ := io.ReadAll(rOut)
    errBytes, _ := io.ReadAll(rErr)

    return string(outBytes), string(errBytes), runErr
}

func panicErr(v any) error { if e, ok := v.(error); ok { return e }; return nil }

func TestIssuesView_WithKey_JSONOutput(t *testing.T) {
    // Fake Linear API
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        b, _ := io.ReadAll(r.Body)
        // Respond to team and issues queries by matching substrings
        q := string(b)
        switch {
        case strings.Contains(q, "teams("):
            w.Write([]byte(`{"data":{"teams":{"nodes":[{"id":"team_1","key":"POK","name":"Pokedex"}]}}}`))
        case strings.Contains(q, "issues("):
            w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"iss_1","identifier":"POK-28","title":"T","description":"D","url":"U","state":{"name":"Todo"}}]}}}`))
        case strings.Contains(q, "issue("):
            w.Write([]byte(`{"data":{"issue":{"id":"iss_1","identifier":"POK-28","title":"T","description":"D","url":"U","state":{"name":"Todo"},"assignee":null,"labels":{"nodes":[]},"project":null}}}`))
        default:
            w.Write([]byte(`{"data":{}}`))
        }
    }))
    defer srv.Close()

    t.Setenv("LINEAR_API_KEY", "test")
    t.Setenv("LINEAR_API_ENDPOINT", srv.URL)

    // Run: linear-cli --json issues view POK-28
    out, _, err := runCLI(t, "--json", "issues", "view", "POK-28")
    if err != nil { t.Fatalf("cli returned error: %v", err) }
    if !regexp.MustCompile(`"identifier":\s*"POK-28"`).MatchString(out) {
        t.Fatalf("expected JSON to contain identifier POK-28, got: %s", out)
    }
}
