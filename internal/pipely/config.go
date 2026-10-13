package pipely

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config retains legacy fields so existing installations can migrate safely.
// New settings keep credentials in the OS credential store or environment.
type Config struct {
	Version      int        `json:"version,omitempty"`
	Auth         string     `json:"auth,omitempty"`
	Contexts     []Context  `json:"contexts,omitempty"`
	Groups       []Group    `json:"groups,omitempty"`
	Org          string     `json:"org"`
	Project      string     `json:"project"`
	PAT          string     `json:"pat,omitempty"`
	LegacyPATOrg string     `json:"-"`
	Repo         string     `json:"repo,omitempty"`
	Branch       string     `json:"branch,omitempty"`
	Pipelines    []Pipeline `json:"pipelines,omitempty"`
}

func LoadConfig(path string) (Config, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return Config{}, e
	}
	var c Config
	e = json.Unmarshal(data, &c)
	if c.PAT != "" {
		c.LegacyPATOrg = c.Org
	}
	return c, e
}

// ListPipelines reads the legacy saved selection. Live discovery uses Client.Pipelines.
func ListPipelines(path string) error {
	c, e := LoadConfig(path)
	if e != nil {
		return e
	}
	for _, p := range c.Pipelines {
		fmt.Printf("%d  %s\n", p.Id, p.Name)
	}
	return nil
}
func GetConfigPath() string {
	dir, e := os.UserConfigDir()
	if e != nil {
		return filepath.Join(".pipely", "config.json")
	}
	return filepath.Join(dir, "pipely", "config.json")
}
