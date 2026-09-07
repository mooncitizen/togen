package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mooncitizen/togen/internal/cli"
)

// Stamped at link time by goreleaser.
var version = "dev"

func main() {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "togen",
		Short:         "Sketch a system, get infrastructure code",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var initProvider, initName string
	var initMigrate bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create togen/ and togen.yml in the current directory",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(func(cwd string) cli.Result { return cli.Init(cwd, initProvider, initName, initMigrate) })
		},
	}
	initCmd.Flags().StringVar(&initProvider, "provider", "aws", "aws, gcp or azure")
	initCmd.Flags().StringVar(&initName, "name", "", "project name")
	initCmd.Flags().BoolVar(&initMigrate, "migrate", false, "rewrite an old togen/togen.json as togen.yml")

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Check togen/project.json",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(cli.Validate)
		},
	}

	var target, out string
	var force bool
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate infrastructure code from togen/project.json",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(func(cwd string) cli.Result { return cli.Generate(cwd, target, out, force) })
		},
	}
	generateCmd.Flags().StringVar(&target, "target", "", "hcl, pulumi or cdktf")
	generateCmd.Flags().StringVar(&out, "out", "", "output directory, default from togen.yml")
	generateCmd.Flags().BoolVar(&force, "force", false, "replace the output directory even if it has files Togen did not write")

	var asJSON bool
	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Estimate the monthly cost of togen/project.json from bundled list prices",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(func(cwd string) cli.Result { return cli.Cost(cwd, asJSON) })
		},
	}
	costCmd.Flags().BoolVar(&asJSON, "json", false, "print the estimate as JSON")

	var simulateJSON bool
	simulateCmd := &cobra.Command{
		Use:   "simulate",
		Short: "Show the load togen/simulation.json puts on every node and edge",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(func(cwd string) cli.Result { return cli.Simulate(cwd, simulateJSON) })
		},
	}
	simulateCmd.Flags().BoolVar(&simulateJSON, "json", false, "print the rates as JSON")

	var port int
	var noOpen bool
	studioCmd := &cobra.Command{
		Use:   "studio",
		Short: "Serve the canvas on 127.0.0.1",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return cli.Studio(cmd.Context(), cwd, port, !noOpen, func(url string) {
				_, _ = fmt.Fprintf(os.Stdout, "togen studio on %s\n", url)
			})
		},
	}
	studioCmd.Flags().IntVar(&port, "port", 3000, "port to listen on, 0 for any free port")
	studioCmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open a browser")

	root.AddCommand(initCmd, validateCmd, generateCmd, costCmd, simulateCmd, studioCmd)
	return root
}

func run(command func(cwd string) cli.Result) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	result := command(cwd)
	if result.Note != "" {
		_, _ = fmt.Fprintln(os.Stderr, result.Note)
	}
	stream := os.Stdout
	if result.Code != 0 {
		stream = os.Stderr
	}
	for _, line := range result.Lines {
		_, _ = fmt.Fprintln(stream, line)
	}
	if result.Code != 0 {
		os.Exit(result.Code)
	}
	return nil
}
