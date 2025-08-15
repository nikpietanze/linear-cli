package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"linear-cli/internal/api"
	"linear-cli/internal/config"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Linear",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login by setting your Linear API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		token, _ := cmd.Flags().GetString("token")
		if token == "" {
			env := os.Getenv("LINEAR_API_KEY")
			if env != "" {
				token = env
			}
		}
		if token == "" {
			fmt.Print("Enter Linear API Key: ")
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println("")
			if err != nil {
				// Fallback to visible input if no TTY
				fmt.Print("Enter Linear API Key (not hidden): ")
				reader := bufio.NewReader(os.Stdin)
				line, rerr := reader.ReadString('\n')
				if rerr != nil {
					return rerr
				}
				token = strings.TrimSpace(line)
			} else {
				token = strings.TrimSpace(string(b))
			}
		}
		if token == "" {
			return errors.New("no token provided")
		}

		cfg, _ := config.Load()
		cfg.APIKey = token
		if err := config.Save(cfg); err != nil {
			return err
		}

		client := api.NewClient(cfg.APIKey)
		viewer, err := client.Viewer()
		if err != nil {
			return fmt.Errorf("saved token, but verification failed: %w", err)
		}
		fmt.Printf("Logged in as %s (%s)\n", viewer.Name, viewer.Email)
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current auth status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()
		if cfg.APIKey == "" {
            if printer(cmd).JSONEnabled() {
                _ = printer(cmd).PrintJSON(map[string]any{"authenticated": false, "message": "Not logged in. Run: linear-cli auth login"})
            } else {
                fmt.Println("Not logged in. Run: linear-cli auth login")
            }
			return nil
		}
		client := api.NewClient(cfg.APIKey)
		viewer, err := client.Viewer()
		if err != nil {
                if printer(cmd).JSONEnabled() {
                    _ = printer(cmd).PrintJSON(map[string]any{"authenticated": false, "error": err.Error()})
                } else {
                    fmt.Println("Token present, but verification failed:", err)
                }
			return nil
		}
        if printer(cmd).JSONEnabled() {
            _ = printer(cmd).PrintJSON(map[string]any{"authenticated": true, "user": viewer})
        } else {
            fmt.Printf("Logged in as %s (%s)\n", viewer.Name, viewer.Email)
        }
		return nil
	},
}

// auth test behaves like status but returns non-zero on failure for CI
var authTestCmd = &cobra.Command{
    Use:   "test",
    Short: "Verify Linear API connectivity and credentials",
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg, _ := config.Load()
        if cfg.APIKey == "" {
            return errors.New("no credentials found: set LINEAR_API_KEY or run 'linear-cli auth login'")
        }
        client := api.NewClient(cfg.APIKey)
        viewer, err := client.Viewer()
        if err != nil {
            return err
        }
        if printer(cmd).JSONEnabled() {
            return printer(cmd).PrintJSON(map[string]any{"ok": true, "user": viewer})
        }
        fmt.Printf("OK: %s (%s)\n", viewer.Name, viewer.Email)
        return nil
    },
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
    authCmd.AddCommand(authTestCmd)
    authLoginCmd.Flags().StringP("token", "t", "", "Linear API key (or set LINEAR_API_KEY)")
}
