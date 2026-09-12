package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchTimeout stops a server that never answers.
const fetchTimeout = 30 * time.Second

// Fetch reads one URL with a GET request.
type Fetch struct {
	// HTTP is the client the request goes through. A nil HTTP means
	// http.DefaultClient.
	HTTP *http.Client
}

func (f Fetch) Name() string { return "fetch" }

func (f Fetch) Description() string {
	return "Fetch a URL with a GET request and return the response body as text."
}

func (f Fetch) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The http or https URL to fetch",
			},
		},
		"required": []string{"url"},
	}
}

func (f Fetch) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		URL string `json:"url"`
	}
	if err := decode(args, &in); err != nil {
		return "", err
	}
	if in.URL == "" {
		return "", errors.New("url is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return "", err
	}
	client := f.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, outputLimit+1))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\n\n%s", resp.Status, truncate(string(body))), nil
}
