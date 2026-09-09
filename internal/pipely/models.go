package pipely

type RunRequest struct {
	Resources struct {
		Repositories struct {
			Self struct {
				RefName string `json:"refName"`
			} `json:"self"`
		} `json:"repositories"`
	} `json:"resources"`
}
