package pipely

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		expectErr bool
	}{
		{
			name: "load valid config",
			config: Config{
				Org:         "TestOrg",
				Project:     "TestProject",
				PAT:         "test-pat",
				Repo:        "TestRepo",
				Branch:      "main",
				PipelineIds: []string{"1", "2", "3"},
			},
			expectErr: false,
		},
		{
			name: "load config with no pipelines",
			config: Config{
				Org:         "TestOrg",
				Project:     "TestProject",
				PAT:         "test-pat",
				Repo:        "TestRepo",
				Branch:      "develop",
				PipelineIds: []string{},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "config.json")

			// Marshal config to JSON and write to file
			data, err := json.Marshal(tt.config)
			if err != nil {
				t.Fatalf("Failed to marshal config: %v", err)
			}

			err = os.WriteFile(tmpFile, data, 0644)
			if err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Load the config
			got, err := LoadConfig(tmpFile)

			if (err != nil) != tt.expectErr {
				t.Errorf("LoadConfig() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if got.Org != tt.config.Org {
				t.Errorf("LoadConfig() Org = %s, want %s", got.Org, tt.config.Org)
			}

			if got.Project != tt.config.Project {
				t.Errorf("LoadConfig() Project = %s, want %s", got.Project, tt.config.Project)
			}

			if got.Branch != tt.config.Branch {
				t.Errorf("LoadConfig() Branch = %s, want %s", got.Branch, tt.config.Branch)
			}

			if len(got.PipelineIds) != len(tt.config.PipelineIds) {
				t.Errorf("LoadConfig() PipelineIds length = %d, want %d", len(got.PipelineIds), len(tt.config.PipelineIds))
			}
		})
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Error("LoadConfig() expected error for non-existent file, got nil")
	}
}

func TestListPipelines(t *testing.T) {
	config := Config{
		Org:         "TestOrg",
		Project:     "TestProject",
		PAT:         "test-pat",
		Repo:        "TestRepo",
		Branch:      "main",
		PipelineIds: []string{"1", "2", "3"},
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	err = os.WriteFile(tmpFile, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Suppress output
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
		w.Close()
	}()

	err = ListPipelines(tmpFile)
	if err != nil {
		t.Errorf("ListPipelines() error = %v", err)
	}
}

func BenchmarkLoadConfig(b *testing.B) {
	config := Config{
		Org:         "TestOrg",
		Project:     "TestProject",
		PAT:         "test-pat-12345",
		Repo:        "TestRepo",
		Branch:      "main",
		PipelineIds: []string{"1", "2", "3", "4", "5"},
	}

	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.json")

	data, _ := json.Marshal(config)
	os.WriteFile(tmpFile, data, 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LoadConfig(tmpFile)
	}
}
