package config

import (
    "errors"
    "os"
    "path/filepath"

    "github.com/BurntSushi/toml"
)

// Config holds user configuration loaded from ~/.config/linear/config.toml
// and environment variables. Environment variables always take precedence.
type Config struct {
    APIKey string `toml:"api_key"`
    TeamPrefs map[string]TeamPrefs `toml:"team_prefs"`
}

// TeamPrefs stores last-used selections per team (keyed by team key, e.g., ENG)
type TeamPrefs struct {
    LastProjectID  string   `toml:"last_project_id"`
    LastAssigneeID string   `toml:"last_assignee_id"`
    LastPriority   int      `toml:"last_priority"`
    LastStateID    string   `toml:"last_state_id"`
    LastTemplate   string   `toml:"last_template"`
    LastLabels     []string `toml:"last_labels"`
}

func configTomlPath() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "linear", "config.toml"), nil
}

// Legacy JSON path used in earlier iterations. We keep a read-only fallback
// for portability across machines that may have the old file.
func legacyJSONPath() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "linear-cli", "config.json"), nil
}

// Load reads configuration from TOML, falling back to legacy JSON if present,
// and finally overlaying environment variables. Missing files are fine.
func Load() (*Config, error) {
    cfg := &Config{}

    // Preferred: TOML at ~/.config/linear/config.toml
    if p, err := configTomlPath(); err == nil {
        if b, err := os.ReadFile(p); err == nil {
            if err := toml.Unmarshal(b, cfg); err != nil {
                return nil, err
            }
        } else if !errors.Is(err, os.ErrNotExist) {
            return nil, err
        }
    } else {
        return nil, err
    }

    // Fallback: legacy JSON path (best-effort). We only parse api_key minimally
    // to avoid adding a JSON dependency here.
    if cfg.APIKey == "" {
        if p, err := legacyJSONPath(); err == nil {
            if b, err := os.ReadFile(p); err == nil {
                // Primitive extraction to avoid pulling in encoding/json solely for fallback.
                // Accepts formats like {"api_key":"..."}
                content := string(b)
                const key = "\"api_key\":"
                if idx := indexInsensitive(content, key); idx >= 0 {
                    // naive slice after key and potential spaces/quotes
                    val := extractJSONStringValue(content[idx+len(key):])
                    if val != "" {
                        cfg.APIKey = val
                    }
                }
            }
        }
    }

    // Environment override
    if v := os.Getenv("LINEAR_API_KEY"); v != "" {
        cfg.APIKey = v
    }
    return cfg, nil
}

// Save writes the configuration to TOML at the preferred path. File mode 0600.
func Save(cfg *Config) error {
    p, err := configTomlPath()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
        return err
    }
    var buf []byte
    buf, err = toml.Marshal(*cfg)
    if err != nil {
        return err
    }
    return os.WriteFile(p, buf, 0o600)
}

// indexInsensitive finds the index of sub in s ignoring ASCII case.
func indexInsensitive(s, sub string) int {
    ls, lsub := len(s), len(sub)
    if lsub == 0 || lsub > ls {
        return -1
    }
    for i := 0; i <= ls-lsub; i++ {
        match := true
        for j := 0; j < lsub; j++ {
            a := s[i+j]
            b := sub[j]
            if a >= 'A' && a <= 'Z' {
                a = a - 'A' + 'a'
            }
            if b >= 'A' && b <= 'Z' {
                b = b - 'A' + 'a'
            }
            if a != b {
                match = false
                break
            }
        }
        if match {
            return i
        }
    }
    return -1
}

// extractJSONStringValue extracts the first JSON string literal value from the
// beginning of s (after a colon), ignoring surrounding spaces.
func extractJSONStringValue(s string) string {
    // skip spaces
    i := 0
    for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\t') {
        i++
    }
    if i >= len(s) || s[i] != '"' {
        return ""
    }
    i++
    start := i
    for i < len(s) {
        if s[i] == '\\' {
            i += 2
            continue
        }
        if s[i] == '"' {
            return s[start:i]
        }
        i++
    }
    return ""
}

// GetConfigDir returns the linear configuration directory
func GetConfigDir() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "linear"), nil
}
