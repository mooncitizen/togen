package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mooncitizen/togen/internal/cli"
	"github.com/mooncitizen/togen/internal/release"
)

// Stamped at link time by goreleaser.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var newReleaseClient = release.NewClient
var releaseCachePath = release.CachePath

func main() {
	root := newRootCommand()
	if err := root.ExecuteContext(context.Background()); err != nil {
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
		PersistentPostRun: func(cmd *cobra.Command, _ []string) {
			if !updateCheckWanted(cmd) {
				return
			}
			path, err := releaseCachePath()
			if err != nil {
				return
			}
			method := func() release.Method {
				method, _, err := release.Detect()
				if err != nil {
					return release.MethodUnknown
				}
				return method
			}
			notice := release.Check(cmd.Context(), newReleaseClient(), path, version, method, time.Now())
			if notice == nil {
				return
			}
			_, _ = fmt.Fprintln(os.Stderr)
			for _, line := range notice.Lines() {
				_, _ = fmt.Fprintln(os.Stderr, line)
			}
		},
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
	var force, strict bool
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate infrastructure code from togen/project.json",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(func(cwd string) cli.Result { return cli.Generate(cwd, target, out, force, strict) })
		},
	}
	generateCmd.Flags().StringVar(&target, "target", "", "hcl, pulumi or cdktf")
	generateCmd.Flags().StringVar(&out, "out", "", "output directory, default from togen.yml")
	generateCmd.Flags().BoolVar(&force, "force", false, "replace the output directory even if it has files Togen did not write")
	generateCmd.Flags().BoolVar(&strict, "strict", false, "fail instead of writing when a node draws but generates nothing")

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

	var versionJSON bool
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version, how it was built and how it was installed",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			method, path, err := release.Detect()
			if err != nil {
				path = "unknown"
			}
			result := cli.Version(cli.VersionInfo{
				Version:  version,
				Commit:   commit,
				Date:     date,
				Go:       runtime.Version(),
				Platform: runtime.GOOS + "/" + runtime.GOARCH,
				Install:  method.String(),
				Path:     path,
			}, versionJSON)
			return emit(result)
		},
	}
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "print the version as JSON")

	var upgradeCheck, upgradeYes bool
	var upgradeVersion string
	upgradeCmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Replace this binary with the latest release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			method, path, err := release.Detect()
			if err != nil {
				return err
			}
			opts := cli.UpgradeOptions{
				Current: version,
				Version: upgradeVersion,
				Check:   upgradeCheck,
				Yes:     upgradeYes,
				Method:  method,
				Path:    path,
			}
			if stderrIsTerminal() {
				opts.Confirm = confirm
			}
			return emit(cli.Upgrade(cmd.Context(), newReleaseClient(), opts))
		},
	}
	upgradeCmd.Flags().BoolVar(&upgradeCheck, "check", false, "report the latest release without installing it")
	upgradeCmd.Flags().BoolVar(&upgradeYes, "yes", false, "do not ask before replacing the binary")
	upgradeCmd.Flags().StringVar(&upgradeVersion, "version", "", "install this tag instead of the latest")

	root.AddCommand(initCmd, validateCmd, generateCmd, costCmd, simulateCmd, studioCmd, versionCmd, upgradeCmd)
	return root
}

func updateCheckWanted(cmd *cobra.Command) bool {
	if !release.Comparable(version) {
		return false
	}
	if os.Getenv("TOGEN_NO_UPDATE_CHECK") != "" || os.Getenv("CI") != "" {
		return false
	}
	if !stderrIsTerminal() {
		return false
	}
	// completion's shell subcommands (zsh, bash, ...) run as cmd.Name() themselves,
	// so completion is only visible by walking up to their parent.
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "upgrade", "version", "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return false
		}
	}
	if flag := cmd.Flags().Lookup("json"); flag != nil && flag.Changed {
		return false
	}
	return true
}

var stderrIsTerminal = func() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func confirm(prompt string) bool {
	_, _ = fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func run(command func(cwd string) cli.Result) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return emit(command(cwd))
}

func emit(result cli.Result) error {
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
