// Command budol-api runs the Budol Bantay Go API on 127.0.0.1:8790.
// TLS and the public hostname come from a reverse proxy or tunnel in front.
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"scam-checker/api"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "budol-api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := api.LoadConfig()
	if cfg.AIMode == "mock" && cfg.Env != "dev" {
		return fmt.Errorf("refusing AI_MODE=mock unless BUDOL_ENV=dev (env=%q)", cfg.Env)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	srv := api.NewServer(cfg, logger, nil)
	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      40 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	logger.Info("listening", "addr", cfg.Listen, "env", cfg.Env)
	return httpSrv.ListenAndServe()
}
