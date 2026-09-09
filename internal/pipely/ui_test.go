package pipely

import (
	"os"
	"testing"
)

func TestSelectProject(t *testing.T) {
	tests := []struct {
		name      string
		projects  []Project
		input     string
		wantIndex int
		wantErr   bool
	}{
		{
			name: "select first project",
			projects: []Project{
				{Name: "Project1"},
				{Name: "Project2"},
			},
			input:     "1\n",
			wantIndex: 0,
		},
		{
			name: "select second project",
			projects: []Project{
				{Name: "Project1"},
				{Name: "Project2"},
			},
			input:     "2\n",
			wantIndex: 1,
		},
		{
			name: "invalid then valid selection",
			projects: []Project{
				{Name: "Project1"},
				{Name: "Project2"},
			},
			input:     "invalid\n1\n",
			wantIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect stdin
			oldStdin := os.Stdin
			oldStdout := os.Stdout
			defer func() {
				os.Stdin = oldStdin
				os.Stdout = oldStdout
			}()

			r, w, _ := os.Pipe()
			os.Stdin = r

			// Suppress output
			_, w2, _ := os.Pipe()
			os.Stdout = w2

			// Write test input
			go func() {
				w.WriteString(tt.input)
				w.Close()
			}()

			got := SelectProject(tt.projects)
			want := tt.projects[tt.wantIndex]

			if got != want {
				t.Errorf("SelectProject() = %v, want %v", got, want)
			}

			w2.Close()
		})
	}
}

func TestSelectRepo(t *testing.T) {
	tests := []struct {
		name      string
		repos     []Repo
		input     string
		wantIndex int
	}{
		{
			name: "select first repo",
			repos: []Repo{
				{Name: "Repo1"},
				{Name: "Repo2"},
			},
			input:     "1\n",
			wantIndex: 0,
		},
		{
			name: "select second repo",
			repos: []Repo{
				{Name: "Repo1"},
				{Name: "Repo2"},
				{Name: "Repo3"},
			},
			input:     "3\n",
			wantIndex: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdin := os.Stdin
			oldStdout := os.Stdout
			defer func() {
				os.Stdin = oldStdin
				os.Stdout = oldStdout
			}()

			r, w, _ := os.Pipe()
			os.Stdin = r

			_, w2, _ := os.Pipe()
			os.Stdout = w2

			go func() {
				w.WriteString(tt.input)
				w.Close()
			}()

			got := SelectRepo(tt.repos)
			want := tt.repos[tt.wantIndex]

			if got != want {
				t.Errorf("SelectRepo() = %v, want %v", got, want)
			}

			w2.Close()
		})
	}
}

func TestSelectPipelines(t *testing.T) {
	tests := []struct {
		name      string
		pipelines []Pipeline
		input     string
		wantCount int
		wantNames []string
	}{
		{
			name: "select single pipeline",
			pipelines: []Pipeline{
				{Name: "Pipeline1"},
				{Name: "Pipeline2"},
			},
			input:     "1\n",
			wantCount: 1,
			wantNames: []string{"Pipeline1"},
		},
		{
			name: "select multiple pipelines",
			pipelines: []Pipeline{
				{Name: "Pipeline1"},
				{Name: "Pipeline2"},
				{Name: "Pipeline3"},
			},
			input:     "1,3\n",
			wantCount: 2,
			wantNames: []string{"Pipeline1", "Pipeline3"},
		},
		{
			name: "select all pipelines",
			pipelines: []Pipeline{
				{Name: "Pipeline1"},
				{Name: "Pipeline2"},
			},
			input:     "1,2\n",
			wantCount: 2,
			wantNames: []string{"Pipeline1", "Pipeline2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdin := os.Stdin
			oldStdout := os.Stdout
			defer func() {
				os.Stdin = oldStdin
				os.Stdout = oldStdout
			}()

			r, w, _ := os.Pipe()
			os.Stdin = r

			_, w2, _ := os.Pipe()
			os.Stdout = w2

			go func() {
				w.WriteString(tt.input)
				w.Close()
			}()

			got := SelectPipelines(tt.pipelines)

			if len(got) != tt.wantCount {
				t.Errorf("SelectPipelines() returned %d items, want %d", len(got), tt.wantCount)
			}

			for i, name := range tt.wantNames {
				if got[i].Name != name {
					t.Errorf("SelectPipelines()[%d].Name = %s, want %s", i, got[i].Name, name)
				}
			}

			w2.Close()
		})
	}
}

func BenchmarkSelectProject(b *testing.B) {
	projects := []Project{
		{Name: "Project1"},
		{Name: "Project2"},
		{Name: "Project3"},
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	// Suppress output
	_, w, _ := os.Pipe()
	os.Stdout = w

	for i := 0; i < b.N; i++ {
		r, pw, _ := os.Pipe()
		os.Stdin = r
		pw.WriteString("1\n")
		pw.Close()

		SelectProject(projects)
	}

	w.Close()
}
