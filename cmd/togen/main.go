package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mooncitizen/togen/internal/cli"
)

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
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var initProvider, initName string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create togen/ in the current directory",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(func(cwd string) cli.Result { return cli.Init(cwd, initProvider, initName) })
		},
	}
	initCmd.Flags().StringVar(&initProvider, "provider", "aws", "aws, gcp or azure")
	initCmd.Flags().StringVar(&initName, "name", "", "project name")

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
	generateCmd.Flags().StringVar(&out, "out", "", "output directory, default from togen/togen.json")
	generateCmd.Flags().BoolVar(&force, "force", false, "replace the output directory even if it has files Togen did not write")

	root.AddCommand(initCmd, validateCmd, generateCmd)
	return root
}

func run(command func(cwd string) cli.Result) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	result := command(cwd)
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
