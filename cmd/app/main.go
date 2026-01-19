// CLI for beat grid analysis and web visualization server.
package main

import (
	"fmt"
	"os"

	"github.com/nzoschke/mixxxlab/pkg/analysis"
	"github.com/nzoschke/mixxxlab/pkg/server"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "app",
	Short: "Beat grid analysis and visualization",
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze <directory>",
	Short: "Analyze audio files and create JSON sidecars",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return runAnalyze(args[0], force)
	},
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start web server on :8080",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServe()
	},
}

func init() {
	analyzeCmd.Flags().BoolP("force", "f", false, "Force re-analysis even if JSON exists")
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(serveCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAnalyze(dir string, force bool) error {
	analyzer, err := analysis.New()
	if err != nil {
		return fmt.Errorf("create analyzer: %w", err)
	}
	defer analyzer.Close()

	return analyzer.AnalyzeDir(dir, force)
}

func runServe() error {
	return server.Run()
}
