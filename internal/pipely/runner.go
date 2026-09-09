package pipely

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
)

func RunPipelines(config Config) {
	pipelineIds := config.PipelineIds
	var wg sync.WaitGroup
	for _, pipelineId := range pipelineIds {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			RunPipeline(id, config)
		}(pipelineId)
	}
	wg.Wait()
}

func RunPipeline(pipelineId string, config Config) {
	slog.Info("Starting pipeline", "pipelineID", pipelineId)
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/pipelines/%s/runs?api-version=7.1", config.Org, config.Project, pipelineId)

	runReq := RunRequest{}
	runReq.Resources.Repositories.Self.RefName = "refs/heads/" + config.Branch

	body, err := json.Marshal(runReq)
	if err != nil {
		slog.Error("Failed to marshal request", "error", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		slog.Error("Failed to create request", "error", err)
		return
	}

	auth := base64.StdEncoding.EncodeToString([]byte(":" + config.PAT))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Failed to execute request", "error", err)
		return
	}

	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	slog.Info("Pipeline run response", "status", resp.Status, "body", string(b))
}
