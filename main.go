package main

import (
    "log"
    "os"

    "github.com/joho/godotenv"
    "linear-cli/cmd"
)

func main() {
    // Load environment variables from .env.local or .env if present.
    // Do not override existing environment variables.
    _ = godotenv.Overload() // loads .env and .env.local, but we prefer not overriding
    // The Overload overrides; to respect existing env, we load explicitly without override order.
    // First .env.local, then .env, only setting keys not already set.
    loadEnvNoOverride(".env.local")
    loadEnvNoOverride(".env")
    cmd.Execute()
}

func loadEnvNoOverride(filename string) {
    m, err := godotenv.Read(filename)
    if err != nil {
        return
    }
    for k, v := range m {
        if _, exists := os.LookupEnv(k); exists {
            continue
        }
        if err := os.Setenv(k, v); err != nil {
            log.Printf("warn: failed setting env %s from %s: %v", k, filename, err)
        }
    }
}
