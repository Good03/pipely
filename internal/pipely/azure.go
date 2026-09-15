package pipely

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func makeRequest(url string, pat string, result interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(":" + pat))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body:", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status: %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

type Project struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ProjectsResponse struct {
	Value []Project `json:"value"`
}

func FetchProjects(org, pat string) ([]Project, error) {
	url := fmt.Sprintf("https://dev.azure.com/%s/_apis/projects?api-version=7.1", org)
	var resp ProjectsResponse
	err := makeRequest(url, pat, &resp)
	return resp.Value, err
}

type Repo struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ReposResponse struct {
	Value []Repo `json:"value"`
}

func FetchRepos(org, project, pat string) ([]Repo, error) {
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories?api-version=7.1", org, project)
	var resp ReposResponse
	err := makeRequest(url, pat, &resp)
	return resp.Value, err
}

type Pipeline struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type PipelinesResponse struct {
	Value []Pipeline `json:"value"`
}

func FetchPipelines(org, project, pat string) ([]Pipeline, error) {
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/pipelines?api-version=7.1", org, project)
	var resp PipelinesResponse
	err := makeRequest(url, pat, &resp)
	return resp.Value, err
}
