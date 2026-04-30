package agentquality

type Case struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Route          string      `json:"route"`
	Input          string      `json:"input"`
	ExpectedTools  []string    `json:"expected_tools,omitempty"`
	AllowedTools   []string    `json:"allowed_tools,omitempty"`
	ExpectedSkills []string    `json:"expected_skills,omitempty"`
	ExpectedAgents []string    `json:"expected_agents,omitempty"`
	Scenario       string      `json:"scenario,omitempty"`
	ExpectedStatus FinalStatus `json:"expected_status"`
	FailureType    FailureType `json:"failure_type,omitempty"`
	Risk           string      `json:"risk,omitempty"`
	Required       bool        `json:"required"`
	Notes          string      `json:"notes,omitempty"`
}
