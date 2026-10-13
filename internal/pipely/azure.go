package pipely

type Project struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}
type Repo struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}
type Pipeline struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Repository *Repo  `json:"repository,omitempty"`
}
