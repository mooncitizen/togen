package cli

import (
	"context"
	"fmt"
	"runtime"

	"github.com/mooncitizen/togen/internal/release"
)

type UpgradeOptions struct {
	Current string
	Version string
	Check   bool
	Yes     bool
	Method  release.Method
	Path    string
	Confirm func(prompt string) bool
}

func Upgrade(ctx context.Context, client release.Client, opts UpgradeOptions) Result {
	if !opts.Check {
		switch opts.Method {
		case release.MethodHomebrew:
			return Result{Code: 0, Lines: []string{
				"This togen came from Homebrew, so Homebrew owns it.",
				"Run `brew upgrade mooncitizen/tap/togen` instead.",
			}}
		case release.MethodNix:
			return Result{Code: 0, Lines: []string{
				"This togen came from nix, and /nix/store is read only.",
				"Update the flake input instead, for instance `nix flake update togen`.",
			}}
		case release.MethodUnknown:
			return Result{Code: 0, Lines: []string{
				fmt.Sprintf("togen cannot write to %s, so it cannot replace itself.", opts.Path),
				"Reinstall it the way you installed it.",
			}}
		}
	}

	rel, err := resolve(ctx, client, opts.Version)
	if err != nil {
		return failure(err)
	}

	upToDate := release.Comparable(opts.Current) && opts.Version == "" && !release.Newer(opts.Current, rel.Tag)
	if opts.Check {
		lines := []string{
			fmt.Sprintf("installed %s (%s)", release.Normalise(opts.Current), opts.Method),
			fmt.Sprintf("latest    %s", rel.Tag),
		}
		if upToDate {
			lines = append(lines, "You are on the latest release.")
		} else {
			lines = append(lines, fmt.Sprintf("Run `%s` to update.", opts.Method.UpgradeCommand()))
		}
		return Result{Code: 0, Lines: lines}
	}
	if upToDate {
		return Result{Code: 0, Lines: []string{fmt.Sprintf("togen %s is the latest release.", release.Normalise(opts.Current))}}
	}

	if !opts.Yes {
		if opts.Confirm == nil {
			return Result{Code: 1, Lines: []string{
				fmt.Sprintf("Refusing to replace %s without a terminal to confirm on.", opts.Path),
				"Pass --yes if you meant this.",
			}}
		}
		prompt := fmt.Sprintf("Replace %s with togen %s?", opts.Path, rel.Tag)
		if !opts.Confirm(prompt) {
			return Result{Code: 0, Lines: []string{"nothing changed"}}
		}
	}

	if err := release.Replace(ctx, client, rel, opts.Path, runtime.GOOS, runtime.GOARCH); err != nil {
		return failure(err)
	}
	return Result{Code: 0, Lines: []string{fmt.Sprintf("togen %s installed at %s", rel.Tag, opts.Path)}}
}

func resolve(ctx context.Context, client release.Client, version string) (release.Release, error) {
	if version == "" {
		return client.Latest(ctx)
	}
	return client.Get(ctx, version)
}
