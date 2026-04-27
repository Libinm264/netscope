package incidents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// JiraConfig holds the credentials for the Jira REST API v3.
type JiraConfig struct {
	BaseURL    string `json:"base_url"`    // e.g. "https://myorg.atlassian.net"
	Email      string `json:"email"`       // Atlassian account email (basic auth user)
	APIToken   string `json:"api_token"`   // Atlassian API token (redacted on read)
	ProjectKey string `json:"project_key"` // e.g. "SEC"
	IssueType  string `json:"issue_type"`  // e.g. "Bug" or "Incident"
}

// CreateJiraIssue opens a Jira issue and returns the issue key (e.g. "SEC-42").
func CreateJiraIssue(cfg JiraConfig, title, description, priority string) (string, error) {
	if cfg.IssueType == "" {
		cfg.IssueType = "Bug"
	}

	jiraPriority := jiraPriorityName(priority)

	payload := map[string]any{
		"fields": map[string]any{
			"project":     map[string]string{"key": cfg.ProjectKey},
			"summary":     title,
			"description": map[string]any{
				"type":    "doc",
				"version": 1,
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{"type": "text", "text": description},
						},
					},
				},
			},
			"issuetype": map[string]string{"name": cfg.IssueType},
			"priority":  map[string]string{"name": jiraPriority},
			"labels":    []string{"netscope", "security"},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST",
		cfg.BaseURL+"/rest/api/3/issue",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(cfg.Email, cfg.APIToken)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("jira request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("jira %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("jira response parse: %w", err)
	}
	return result.Key, nil
}

func jiraPriorityName(severity string) string {
	switch severity {
	case "critical":
		return "Highest"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	default:
		return "Low"
	}
}
