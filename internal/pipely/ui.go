package pipely

import (
	"fmt"
	"strconv"
	"strings"
)

// SelectProject lets user pick one Project
func SelectProject(projects []Project) Project {
	for {
		fmt.Println("Select a project:")
		for i, p := range projects {
			fmt.Printf("%d: %s\n", i+1, p.Name)
		}
		fmt.Print("Enter number: ")
		var input string
		fmt.Scan(&input)
		idx, err := strconv.Atoi(input)
		if err == nil && idx > 0 && idx <= len(projects) {
			return projects[idx-1]
		}
		fmt.Println("Invalid selection, try again.")
	}
}

// SelectRepo lets user pick one Repo
func SelectRepo(repos []Repo) Repo {
	for {
		fmt.Println("Select a repo:")
		for i, r := range repos {
			fmt.Printf("%d: %s\n", i+1, r.Name)
		}
		fmt.Print("Enter number: ")
		var input string
		fmt.Scan(&input)
		idx, err := strconv.Atoi(input)
		if err == nil && idx > 0 && idx <= len(repos) {
			return repos[idx-1]
		}
		fmt.Println("Invalid selection, try again.")
	}
}

// SelectPipelines lets user pick multiple Pipelines
func SelectPipelines(pipelines []Pipeline) []Pipeline {
	for {
		fmt.Println("Select pipelines (comma separated numbers):")
		for i, p := range pipelines {
			fmt.Printf("%d: %s\n", i+1, p.Name)
		}
		fmt.Print("Enter numbers: ")
		var input string
		fmt.Scan(&input)
		parts := strings.Split(input, ",")
		var selected []Pipeline
		valid := true
		for _, p := range parts {
			idx, err := strconv.Atoi(strings.TrimSpace(p))
			if err == nil && idx > 0 && idx <= len(pipelines) {
				selected = append(selected, pipelines[idx-1])
			} else {
				valid = false
				break
			}
		}
		if valid && len(selected) > 0 {
			return selected
		}
		fmt.Println("Invalid selection, try again.")
	}
}
