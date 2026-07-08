package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sgbudje/runright-platform/internal/server"
)

var rootCmd = &cobra.Command{
	Use:   "runright-server",
	Short: "RunRight platform server",
	Long:  `runright-server runs the RunRight backend API and serves the dashboard.`,
}

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the RunRight dashboard backend",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "HTTP port")
	rootCmd.AddCommand(serveCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServe(_ *cobra.Command, _ []string) error {
	cfg := server.ConfigFromEnv()
	cfg.Port = servePort
	srv, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("server init: %w", err)
	}
	return srv.Run(servePort)
}
