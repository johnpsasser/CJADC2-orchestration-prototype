// Package opa provides a client for interacting with Open Policy Agent
package opa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an OPA API client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new OPA client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Decision represents an OPA policy decision
type Decision struct {
	Allowed    bool                   `json:"allowed"`
	Reasons    []string               `json:"reasons,omitempty"`
	Violations []string               `json:"violations,omitempty"`
	Warnings   []string               `json:"warnings,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// QueryInput is the input for an OPA query
type QueryInput struct {
	Input interface{} `json:"input"`
}

// QueryResult is the result of an OPA query
type QueryResult struct {
	Result map[string]interface{} `json:"result"`
}

// Query evaluates a policy and returns the result
func (c *Client) Query(ctx context.Context, path string, input interface{}) (*QueryResult, error) {
	url := fmt.Sprintf("%s/v1/data/%s", c.baseURL, path)

	body, err := json.Marshal(QueryInput{Input: input})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OPA returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result QueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// Decide evaluates a policy and returns a structured decision
func (c *Client) Decide(ctx context.Context, policyPath string, input interface{}) (*Decision, error) {
	result, err := c.Query(ctx, policyPath, input)
	if err != nil {
		return nil, err
	}

	decision := &Decision{
		Allowed:  false,
		Metadata: make(map[string]interface{}),
	}

	// Extract decision fields from result
	if result.Result != nil {
		// Check multiple possible field names for allowed status
		if allowed, ok := result.Result["allow"].(bool); ok {
			decision.Allowed = allowed
		} else if allowed, ok := result.Result["allow_effect"].(bool); ok {
			decision.Allowed = allowed
		} else if allowed, ok := result.Result["allowed"].(bool); ok {
			decision.Allowed = allowed
		} else if decisionObj, ok := result.Result["decision"].(map[string]interface{}); ok {
			if allowed, ok := decisionObj["allowed"].(bool); ok {
				decision.Allowed = allowed
			}
		}

		// Check multiple possible field names for deny reasons
		if reasons, ok := result.Result["deny"].([]interface{}); ok {
			for _, r := range reasons {
				if s, ok := r.(string); ok {
					decision.Reasons = append(decision.Reasons, s)
				}
			}
		} else if reasons, ok := result.Result["deny"].(map[string]interface{}); ok {
			for _, r := range reasons {
				if s, ok := r.(string); ok {
					decision.Reasons = append(decision.Reasons, s)
				}
			}
		}

		if warnings, ok := result.Result["warnings"].([]interface{}); ok {
			for _, w := range warnings {
				if s, ok := w.(string); ok {
					decision.Warnings = append(decision.Warnings, s)
				}
			}
		}

		// Store full result as metadata
		decision.Metadata["raw_result"] = result.Result
	}

	return decision, nil
}

// CheckOrigin validates message origin using the origin attestation policy
func (c *Client) CheckOrigin(ctx context.Context, envelope interface{}) (*Decision, error) {
	input := map[string]interface{}{
		"envelope":             envelope,
		"skip_signature_check": true, // For MVP, skip signature verification
	}
	return c.Decide(ctx, "cjadc2/origin", input)
}

// CheckDataHandling validates data handling using the data handling policy
func (c *Client) CheckDataHandling(ctx context.Context, agentID, agentType string, data interface{}) (*Decision, error) {
	input := map[string]interface{}{
		"agent_id":           agentID,
		"agent_type":         agentType,
		"data":               data,
		"audit_enabled":      true,
		"encryption_enabled": false, // MVP doesn't use encryption
	}
	return c.Decide(ctx, "cjadc2/data_handling", input)
}

// CheckProposal validates an action proposal
func (c *Client) CheckProposal(ctx context.Context, proposal interface{}, track interface{}, trackExists bool, pendingProposals []interface{}) (*Decision, error) {
	input := map[string]interface{}{
		"proposal":          proposal,
		"track":             track,
		"track_exists":      trackExists,
		"pending_proposals": pendingProposals,
	}
	return c.Decide(ctx, "cjadc2/proposals", input)
}

// CheckEffectRelease validates that an effect can be released
func (c *Client) CheckEffectRelease(ctx context.Context, decision, proposal interface{}, actionType string, alreadyExecuted bool) (*Decision, error) {
	input := map[string]interface{}{
		"decision":         decision,
		"proposal":         proposal,
		"action_type":      actionType,
		"already_executed": alreadyExecuted,
	}
	return c.Decide(ctx, "cjadc2/effects", input)
}

// Health checks if OPA is healthy
func (c *Client) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OPA unhealthy: status %d", resp.StatusCode)
	}

	return nil
}
