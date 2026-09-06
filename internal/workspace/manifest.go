package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const manifestName = ".togen-manifest.json"

var terraformNames = map[string]bool{
	".terraform":                   true,
	".terraform.lock.hcl":          true,
	"terraform.tfstate":            true,
	"terraform.tfstate.backup":     true,
	"terraform.tfstate.d":          true,
	".terraform.tfstate.lock.info": true,
}

func isTfvars(name string) bool { return strings.HasSuffix(name, ".tfvars") }

func isTerraformFile(name string) bool { return terraformNames[name] || isTfvars(name) }

func isIgnored(name string) bool { return name == manifestName || isTerraformFile(name) }

type manifest struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
}

func listFiles(dir, base string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if isTfvars(entry.Name()) || dir == base && isIgnored(entry.Name()) {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			nested, err := listFiles(full, base)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
			continue
		}
		rel, err := filepath.Rel(base, full)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out, nil
}

func readManifest(dir string) map[string]bool {
	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return nil
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	known := make(map[string]bool, len(m.Files))
	for _, f := range m.Files {
		known[f] = true
	}
	return known
}

func checkOutDir(dir string) ([]string, error) {
	if !Exists(dir) {
		return nil, nil
	}
	present, err := listFiles(dir, dir)
	if err != nil {
		return nil, err
	}
	if len(present) == 0 {
		return nil, nil
	}
	known := readManifest(dir)
	var strangers []string
	for _, f := range present {
		if !known[f] {
			strangers = append(strangers, f)
		}
	}
	slices.Sort(strangers)
	return strangers, nil
}

// A crash between the two swap renames leaves .togen-old behind, either instead of the
// output directory or beside it. Either way the Terraform files in it are the only copy.
func recoverOld(old, dir string) error {
	if !Exists(old) {
		return nil
	}
	if !Exists(dir) {
		return os.Rename(old, dir)
	}
	if err := moveTerraformFiles(old, dir, true); err != nil {
		return err
	}
	return os.RemoveAll(old)
}

// The named working files only mean something at the top of the output
// directory; a tfvars file is kept wherever it sits.
func moveTerraformFiles(from, to string, top bool) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		src, dst := filepath.Join(from, name), filepath.Join(to, name)
		keep := isTfvars(name) || top && terraformNames[name]
		switch {
		case keep && !Exists(dst):
			if err := os.MkdirAll(to, 0o755); err != nil {
				return err
			}
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		case !keep && entry.IsDir():
			if err := moveTerraformFiles(src, dst, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeOutputs(dir string, files map[string][]byte) error {
	tmp := dir + ".togen-tmp"
	old := dir + ".togen-old"
	if err := recoverOld(old, dir); err != nil {
		return err
	}
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}

	names := sortedKeys(files)
	for _, name := range names {
		path := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return err
		}
	}
	if err := WriteJSONFile(filepath.Join(tmp, manifestName), manifest{Version: 1, Files: names}); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	hadOld := Exists(dir)
	if hadOld {
		if err := os.Rename(dir, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		return err
	}
	if !hadOld {
		return nil
	}
	if err := moveTerraformFiles(old, dir, true); err != nil {
		return err
	}
	return os.RemoveAll(old)
}

func sortedKeys(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
