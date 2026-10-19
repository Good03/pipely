package pipely

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

type Context struct {
	Org     string `json:"org"`
	Project string `json:"project"`
}
type Group struct {
	Name    string       `json:"name"`
	Org     string       `json:"org"`
	Project string       `json:"project"`
	Branch  string       `json:"branch,omitempty"`
	Members []RunOptions `json:"members"`
}

func ReadSettings(path string) (Config, error) {
	c, err := LoadConfig(path)
	if os.IsNotExist(err) {
		return Config{Version: 2, Auth: "az", Branch: "main"}, nil
	}
	if err != nil {
		return c, err
	}
	if c.Auth == "" {
		if c.PAT != "" {
			c.Auth = "pat"
		} else {
			c.Auth = "az"
		}
	}
	if c.Branch == "" {
		c.Branch = "main"
	}
	if c.Org != "" && c.Project != "" {
		c.RememberContext(c.Org, c.Project)
	}
	return c, nil
}

func (c *Config) RememberContext(org, project string) {
	for _, x := range c.Contexts {
		if x.Org == org && x.Project == project {
			c.Org = org
			c.Project = project
			return
		}
	}
	c.Contexts = append(c.Contexts, Context{org, project})
	c.Org = org
	c.Project = project
}

func SaveSettings(path string, c Config) error {
	// Migrate a legacy plaintext PAT before removing it from the configuration.
	if c.PAT != "" {
		origin := c.LegacyPATOrg
		if origin == "" {
			origin = c.Org
		}
		if err := StorePAT(origin, c.PAT); err != nil {
			return fmt.Errorf("migrate legacy PAT to credential store: %w; configuration was not changed", err)
		}
		c.PAT = ""
	}
	c.Version = 2
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pipely-*.json")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func StorePAT(org, pat string) error {
	if strings.TrimSpace(org) == "" || strings.TrimSpace(pat) == "" {
		return fmt.Errorf("organization and PAT are required")
	}
	return keyring.Set("pipely", org, pat)
}
func FindGroup(c Config, name string) (Group, error) {
	for _, g := range c.Groups {
		if g.Name == name && g.Org == c.Org && g.Project == c.Project {
			return g, nil
		}
	}
	return Group{}, fmt.Errorf("group %q not found in %s/%s", name, c.Org, c.Project)
}
func PutGroup(c *Config, g Group) error {
	if strings.TrimSpace(g.Name) == "" || g.Org == "" || g.Project == "" || len(g.Members) == 0 {
		return fmt.Errorf("group name, context and at least one pipeline are required")
	}
	seen := map[int]bool{}
	for _, m := range g.Members {
		if m.PipelineID <= 0 || seen[m.PipelineID] {
			return fmt.Errorf("pipeline IDs must be positive and unique")
		}
		seen[m.PipelineID] = true
	}
	for i, x := range c.Groups {
		if x.Name == g.Name && x.Org == g.Org && x.Project == g.Project {
			c.Groups[i] = g
			return nil
		}
	}
	c.Groups = append(c.Groups, g)
	return nil
}
