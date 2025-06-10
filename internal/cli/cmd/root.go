package cmd

import (
	"fmt"
	"os"

	subcmd "github.com/sap/crossplane-provider-btp/internal/cli/cmd/subcmd"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "xpbtp",
	Short: "Crossplane-BTP-Importing (XPBTP)",
	Long:  "XPBTP (Crossplane-BTP-Importing) is a CLI tool to import pre-existing BTP resources into your ManagedControlPlane (MCP)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Welcome to XPBTP! Use --help for more information.")
	},
}

// Execute runs the root command and handles errors
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Initialize subcommands
	subcmd.AddInitCMD(rootCmd)
	subcmd.AddImportCMD(rootCmd)
	subcmd.AddToolingCMD(rootCmd)
}
