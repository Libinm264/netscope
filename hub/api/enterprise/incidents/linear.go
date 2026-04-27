package incidents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LinearConfig holds the credentials for the Linear GraphQL API.
type LinearConfig struct {
	APIKey   string `json:"api_key"`   // Personal or OAuth token (redacted on read)
	TeamID   string `json:"team_id"`   // Linear team UUID
	Priority int    `json:"priority"`  // 0=no priority, 1=urgent, 2=high, 3=medium, 4=low
}

const linearGraphQLEndpoint = "https://api.linear.app/graphql"

// CreateLinearIssue creates a Linear issue and returns the issue identifier (e.g. "SEC-42").
func CreateLinearIssue(cfg LinearConfig, title, description, severity string) (string, error) {
	priority := linearPriority(cfg.Priority, severity)

	mutation := `
		mutation CreateIssue($input: IssueCreateInput!) {
			issueCreate(input: $input) {
				success
				issue {
					id
					identifier
				}
			}
		}`

	variables := map[string]any{
		"input": map[string]any{
			"teamId":      cfg.TeamID,
			"title":       title,
			"description": description,
			"priority":    priority,
			"labelIds":    []string{},
		},
	}

	payload := map[string]any{
		"query":     mutation,
		"variables": variables,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", linearGraphQLEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", cfg.APIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("linear request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linear %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   struct {
					ID         string `json:"id"`
					Identifier string `json:"identifier"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("linear parse: %w", err)
	}
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("linear error: %s", result.Errors[0].Message)
	}
	if !result.Data.IssueCreate.Success {
		return "", fmt.Errorf("linear: issue creation failed")
	}
	return result.Data.IssueCreate.Issue.Identifier, nil
}

func linearPriority(configured int, severity string) int {
	if configured > 0 {
		return configured
	}
	switch severity {
	case "critical":
		return 1 // Urgent
	case "high":
		return 2 // High
	case "medium":
		return 3 // Medium
	default:
		return 4 // Low
	}
}
