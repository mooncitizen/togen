// What each provider offers, as data. The schema, the styles, the relation table, the
// usage keys and the studio's palette are all derived from these files (ADR 0001, ADR 0007).
package catalogue

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

//go:embed data/*.json
var data embed.FS

type Tier string

const (
	TierGenerates Tier = "generates"
	TierDraws     Tier = "draws"
)

var Tiers = []Tier{TierGenerates, TierDraws}

type Role string

const (
	RoleEntry     Role = "entry"
	RoleCompute   Role = "compute"
	RoleStore     Role = "store"
	RoleMessaging Role = "messaging"
)

var Roles = []Role{RoleEntry, RoleCompute, RoleStore, RoleMessaging}

type Entry struct {
	ID          string
	Label       string
	Group       string
	Description string
	Resource    string
	Aliases     []string
	Roles       []Role
	Usage       []string
	Tier        Tier
	Style       Look
	// Properties carries a draw-only entry's JSON Schema. An entry that generates has a
	// props struct in internal/ir instead, and this is nil.
	Properties map[string]any
}

func (e Entry) HasRole(r Role) bool { return slices.Contains(e.Roles, r) }

// Portable means every provider declares the id, which is what a bare id promises.
func (e Entry) Portable() bool { return portable(e.ID) }

func portable(id string) bool { return !strings.Contains(id, "/") }

type Catalogue struct {
	providers []string
	entries   map[string][]Entry
	schemes   map[string]Scheme
}

func (c *Catalogue) Providers() []string { return slices.Clone(c.providers) }

// For returns the provider's entries in catalogue order, which is the order the palette
// shows them in within a group.
func (c *Catalogue) For(provider string) []Entry { return slices.Clone(c.entries[provider]) }

func (c *Catalogue) Lookup(provider, id string) (Entry, bool) {
	for _, e := range c.entries[provider] {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

func (c *Catalogue) Scheme(provider string) (Scheme, bool) {
	s, ok := c.schemes[provider]
	return s, ok
}

func (c *Catalogue) Schemes() map[string]Scheme { return c.schemes }

// All returns every entry any provider declares, core entries first in core order, then
// each provider's own in provider order. Callers that publish one provider-agnostic
// document (the project schema, the relation table) work from this.
func (c *Catalogue) All() []Entry {
	var out []Entry
	seen := map[string]bool{}
	for _, provider := range c.providers {
		for _, e := range c.entries[provider] {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	return out
}

func (c *Catalogue) IDs() []string {
	entries := c.All()
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

// WithRole lists the ids playing a role, in All order, which is what keeps the derived
// relation table in the order it was written by hand.
func (c *Catalogue) WithRole(r Role) []string {
	var out []string
	for _, e := range c.All() {
		if e.HasRole(r) && !slices.Contains(out, e.ID) {
			out = append(out, e.ID)
		}
	}
	return out
}

func (c *Catalogue) Icons() []string {
	var ids []string
	for _, provider := range c.providers {
		for _, e := range c.entries[provider] {
			ids = append(ids, e.Style.Icon)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

var embedded = mustLoad(data, "data")

func Embedded() *Catalogue { return embedded }

func mustLoad(fsys fs.FS, dir string) *Catalogue {
	c, err := Load(fsys, dir)
	if err != nil {
		panic(err)
	}
	return c
}

// The order providers are merged in, which decides All order and so the published
// documents. A provider file that is not here is not read.
var providerOrder = []string{"aws", "gcp", "azure"}

func Load(fsys fs.FS, dir string) (*Catalogue, error) {
	var core coreFile
	if err := readJSON(fsys, path.Join(dir, "core.json"), &core); err != nil {
		return nil, err
	}
	shared := map[string]rawEntry{}
	for _, e := range core.Entries {
		if _, dup := shared[e.ID]; dup {
			return nil, fmt.Errorf("core.json declares '%s' twice", e.ID)
		}
		shared[e.ID] = e
	}

	c := &Catalogue{entries: map[string][]Entry{}, schemes: map[string]Scheme{}}
	for _, provider := range providerOrder {
		var file providerFile
		if err := readJSON(fsys, path.Join(dir, provider+".json"), &file); err != nil {
			return nil, err
		}
		if file.Provider != provider {
			return nil, fmt.Errorf("%s.json says its provider is '%s'", provider, file.Provider)
		}
		entries, err := merge(provider, file, shared)
		if err != nil {
			return nil, err
		}
		c.providers = append(c.providers, provider)
		c.entries[provider] = entries
		c.schemes[provider] = scheme(file, entries)
	}
	if err := c.checkPortable(shared); err != nil {
		return nil, err
	}
	return c, nil
}

func merge(provider string, file providerFile, shared map[string]rawEntry) ([]Entry, error) {
	var out []Entry
	seen := map[string]bool{}
	for _, raw := range file.Entries {
		if seen[raw.ID] {
			return nil, fmt.Errorf("%s.json declares '%s' twice", provider, raw.ID)
		}
		seen[raw.ID] = true

		entry := raw
		if base, ok := shared[raw.ID]; ok {
			entry = base.overlaidWith(raw)
		} else if portable(raw.ID) {
			return nil, fmt.Errorf("%s.json declares '%s', which core.json has not got", provider, raw.ID)
		}
		e, err := entry.entry(provider)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// A bare id promises every provider has it, so a core entry a provider is missing is a
// mistake in the data rather than a gap a user should discover.
func (c *Catalogue) checkPortable(shared map[string]rawEntry) error {
	for id := range shared {
		for _, provider := range c.providers {
			if _, ok := c.Lookup(provider, id); !ok {
				return fmt.Errorf("core.json declares '%s', which %s.json has not got", id, provider)
			}
		}
	}
	return nil
}

func readJSON(fsys fs.FS, name string, into any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
