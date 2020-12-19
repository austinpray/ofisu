package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string
var logger *log.Logger

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ofisu",
	Short: "Emulates a physical office",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// TODO: json logging in prod
	logger = log.New(os.Stderr, "", log.LUTC)
}
