package catalogue

import (
	"fmt"
	"slices"
)

type coreFile struct {
	Entries []rawEntry `json:"entries"`
}

type providerFile struct {
	Provider      string     `json:"provider"`
	Accent        string     `json:"accent"`
	Network       Boundary   `json:"network"`
	ResourceGroup *Boundary  `json:"resourceGroup,omitempty"`
	Entries       []rawEntry `json:"entries"`
}

type rawEntry struct {
	ID          string         `json:"id"`
	Label       string         `json:"label,omitempty"`
	Group       string         `json:"group,omitempty"`
	Description string         `json:"description,omitempty"`
	Aliases     []string       `json:"aliases,omitempty"`
	Roles       []Role         `json:"roles,omitempty"`
	Usage       []string       `json:"usage,omitempty"`
	Tier        Tier           `json:"tier,omitempty"`
	Resource    string         `json:"resource,omitempty"`
	Style       *Look          `json:"style,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
}

// A provider entry says the concrete half; anything it leaves out comes from core.
func (base rawEntry) overlaidWith(over rawEntry) rawEntry {
	out := base
	out.Tier = over.Tier
	out.Resource = over.Resource
	out.Style = over.Style
	if over.Label != "" {
		out.Label = over.Label
	}
	if over.Group != "" {
		out.Group = over.Group
	}
	if over.Description != "" {
		out.Description = over.Description
	}
	if over.Aliases != nil {
		out.Aliases = over.Aliases
	}
	if over.Roles != nil {
		out.Roles = over.Roles
	}
	if over.Usage != nil {
		out.Usage = over.Usage
	}
	if over.Properties != nil {
		out.Properties = over.Properties
	}
	return out
}

func (r rawEntry) entry(provider string) (Entry, error) {
	where := fmt.Sprintf("%s entry '%s'", provider, r.ID)
	switch {
	case r.ID == "":
		return Entry{}, fmt.Errorf("%s.json has an entry with no id", provider)
	case r.Label == "":
		return Entry{}, fmt.Errorf("%s has no label", where)
	case r.Group == "":
		return Entry{}, fmt.Errorf("%s has no group", where)
	case r.Description == "":
		return Entry{}, fmt.Errorf("%s has no description", where)
	case r.Resource == "":
		return Entry{}, fmt.Errorf("%s has no resource name", where)
	case r.Style == nil:
		return Entry{}, fmt.Errorf("%s has no style", where)
	case !slices.Contains(Tiers, r.Tier):
		return Entry{}, fmt.Errorf("%s has tier '%s', which is not one of generates or draws", where, r.Tier)
	case len(r.Roles) == 0:
		return Entry{}, fmt.Errorf("%s has no roles", where)
	}
	for _, role := range r.Roles {
		if !slices.Contains(Roles, role) {
			return Entry{}, fmt.Errorf("%s has role '%s', which is not one of %v", where, role, Roles)
		}
	}
	if err := r.Style.check(where); err != nil {
		return Entry{}, err
	}
	if r.Tier == TierGenerates && r.Properties != nil {
		return Entry{}, fmt.Errorf("%s generates, so its properties come from its props struct, not the catalogue", where)
	}
	return Entry{
		ID:          r.ID,
		Label:       r.Label,
		Group:       r.Group,
		Description: r.Description,
		Resource:    r.Resource,
		Aliases:     r.Aliases,
		Roles:       r.Roles,
		Usage:       r.Usage,
		Tier:        r.Tier,
		Style:       *r.Style,
		Properties:  r.Properties,
	}, nil
}

func scheme(file providerFile, entries []Entry) Scheme {
	kinds := make(map[string]Kind, len(entries))
	for _, e := range entries {
		kinds[e.ID] = Kind{Color: e.Style.Color, Icon: e.Style.Icon, Shape: e.Style.Shape, Resource: e.Resource}
	}
	return Scheme{
		Accent:        file.Accent,
		Network:       file.Network,
		ResourceGroup: file.ResourceGroup,
		Kinds:         kinds,
	}
}
