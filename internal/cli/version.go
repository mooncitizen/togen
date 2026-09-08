package cli

import (
	"encoding/json"
	"fmt"
)

type VersionInfo struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Date     string `json:"date"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
	Install  string `json:"install"`
	Path     string `json:"path"`
}

func Version(info VersionInfo, asJSON bool) Result {
	if asJSON {
		raw, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return failure(err)
		}
		return Result{Code: 0, Lines: []string{string(raw)}}
	}
	return Result{Code: 0, Lines: []string{
		fmt.Sprintf("togen %s", info.Version),
		fmt.Sprintf("commit  %s", info.Commit),
		fmt.Sprintf("built   %s", info.Date),
		fmt.Sprintf("go      %s %s", info.Go, info.Platform),
		fmt.Sprintf("install %s (%s)", info.Install, info.Path),
	}}
}
