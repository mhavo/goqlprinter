// Package cmd implements the CLI commands for the goqlprinter application.
package cmd

import (
	"os"

	"goqlprinter/brotherql"
	icfg "goqlprinter/internal/config"
	"goqlprinter/internal/logging"

	"github.com/spf13/cobra"
)

var (
	// Global state shared across subcommands.
	Cfg             *icfg.Config
	BackendProvider brotherql.BackendProvider

	// CLI flags
	flagPrinter string
	flagModel   string

	// defaultCommand runs when the binary is started without arguments
	// (set by the gui build so a double-clicked app opens its window).
	defaultCommand string
)

var rootCmd = &cobra.Command{
	Use:   "goqlprinter",
	Short: "Brother QL label printer CLI and server",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip init for serve and gui commands (they do their own setup).
		if cmd.Name() == "serve" || cmd.Name() == "gui" {
			return nil
		}

		logLevel := os.Getenv("LOG_LEVEL")
		if logLevel == "" {
			logLevel = "WARN"
		}
		logging.Init(logLevel)

		cfg, err := icfg.LoadConfig()
		if err != nil {
			return err
		}
		Cfg = cfg

		BackendProvider = InitBackendProvider(cfg)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagPrinter, "printer", "p", "", "printer name or URI (e.g. QL-800, usb:001:005)")
	rootCmd.PersistentFlags().StringVarP(&flagModel, "model", "m", "", "printer model override")
}

// Execute runs the root command.
func Execute() {
	if defaultCommand != "" && len(os.Args) == 1 {
		rootCmd.SetArgs([]string{defaultCommand})
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
