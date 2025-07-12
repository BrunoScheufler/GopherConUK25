package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/google/uuid"
)

type RestAPIClient struct {
	baseURL    string
	httpClient *http.Client
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewRestAPIClient(baseURL string) *RestAPIClient {
	return &RestAPIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *RestAPIClient) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// Account operations

func (c *RestAPIClient) ListAccounts(ctx context.Context) ([]store.Account, error) {
	var accounts []store.Account
	err := c.doRequest(ctx, "GET", "/accounts", nil, &accounts)
	return accounts, err
}

func (c *RestAPIClient) CreateAccount(ctx context.Context, account store.Account) (*store.Account, error) {
	var result store.Account
	err := c.doRequest(ctx, "POST", "/accounts", account, &result)
	return &result, err
}

func (c *RestAPIClient) UpdateAccount(ctx context.Context, account store.Account) (*store.Account, error) {
	var result store.Account
	path := fmt.Sprintf("/accounts/%s", account.ID.String())
	err := c.doRequest(ctx, "PUT", path, account, &result)
	return &result, err
}

// Note operations

func (c *RestAPIClient) ListNotes(ctx context.Context, accountID uuid.UUID) ([]store.Note, error) {
	var notes []store.Note
	path := fmt.Sprintf("/accounts/%s/notes", accountID.String())
	err := c.doRequest(ctx, "GET", path, nil, &notes)
	return notes, err
}

func (c *RestAPIClient) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	var note store.Note
	path := fmt.Sprintf("/accounts/%s/notes/%s", accountID.String(), noteID.String())
	err := c.doRequest(ctx, "GET", path, nil, &note)
	return &note, err
}

func (c *RestAPIClient) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) (*store.Note, error) {
	var result store.Note
	path := fmt.Sprintf("/accounts/%s/notes", accountID.String())
	err := c.doRequest(ctx, "POST", path, note, &result)
	return &result, err
}

func (c *RestAPIClient) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) (*store.Note, error) {
	var result store.Note
	path := fmt.Sprintf("/accounts/%s/notes/%s", accountID.String(), note.ID.String())
	err := c.doRequest(ctx, "PUT", path, note, &result)
	return &result, err
}

func (c *RestAPIClient) DeleteNote(ctx context.Context, accountID, noteID uuid.UUID) error {
	path := fmt.Sprintf("/accounts/%s/notes/%s", accountID.String(), noteID.String())
	return c.doRequest(ctx, "DELETE", path, nil, nil)
}