package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

type Config struct {
	Org         string `json:"org"`
	Project     string `json:"project"`
	PAT         string `json:"pat"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	PipelineIds []int  `json:"pipeline_ids"`
}

type RunRequest struct {
	Resources struct {
		Repositories struct {
			Self struct {
				RefName string `json:"refName"`
			} `json:"self"`
		} `json:"repositories"`
	} `json:"resources"`
}

//TODO: Create a config while where all reusable data will be located
// for example: azure devops org, url, repo, pipeline configuration etc

//TODO: Split init part to request only org and project so I can get data from azure devops api and extctract repos, branches and pipelines to create a config file with all the data needed to run pipelines

//TODO: Make a CLI commands (update config, run pipeline/pipelines, list pipelines, list branches, list repos, set repo, set branch)

//TODO: Create flags to set repo, branch, pipeline id and run pipelines with those flags without the need

const fileName = "config.json"
const dirName = "pipely"
const path = "path"

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func getConfig(path string) Config {
	dataFromFile, _ := os.ReadFile(path)
	config := Config{}
	err := json.Unmarshal(dataFromFile, &config)
	fmt.Println(string(dataFromFile))
	check(err)

	return config
}

func runPipelines(config Config) {
	pipelineIds := config.PipelineIds
	var wg sync.WaitGroup
	for _, pipelineId := range pipelineIds {
		wg.Go(func() {
			fmt.Println("Starting pipeline " + strconv.Itoa(pipelineId))
			url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/pipelines/%d/runs?api-version=7.1", config.Org, config.Project, pipelineId)

			runReq := RunRequest{}
			runReq.Resources.Repositories.Self.RefName = "refs/heads/" + config.Branch

			body, err := json.Marshal(runReq)
			check(err)

			req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
			check(err)

			auth := base64.StdEncoding.EncodeToString([]byte(":" + config.PAT))
			req.Header.Set("Authorization", "Basic "+auth)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			check(err)

			defer resp.Body.Close()

			b, _ := io.ReadAll(resp.Body)

			fmt.Println("Status:", resp.Status)
			fmt.Println(string(b))
		})
	}
	wg.Wait()
}

func main() {
	path := filepath.Join(path, fileName)

	log.Printf("Checking %s existence...\n", path)
	if !fileExists(path) {
		log.Printf("Config does not exist, creating a config in %s\n", path)
		var project string
		fmt.Print("Enter Project name: ")
		fmt.Scan(&project)
		var pat string
		fmt.Print("Enter PAT: ")
		fmt.Scan(&pat)
		config := Config{
			Org:         "org",
			Project:     project,
			PipelineIds: []int{73768, 73778},
			PAT:         pat,
			Repo:        "org",
			Branch:      "test/pipely",
		}
		configJson, _ := json.Marshal(config)
		err := os.WriteFile(path, configJson, 0777)
		check(err)
	}
	log.Printf("%s does exist\n", path)

	config := getConfig(path)

	runPipelines(config)
}
