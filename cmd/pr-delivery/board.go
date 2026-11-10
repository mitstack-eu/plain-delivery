package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var errNoBoard = errors.New("no board to read, since TRACKER_BASE_URL is unset")

type issueList struct {
	Issues []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"issues"`
}

// boardStatuses reads every key's status in one request to the board's issue list, asking for the status field alone.
func boardStatuses(baseURL, token string) statusReader {
	return func(keys []string) (map[string]string, error) {
		if baseURL == "" {
			return nil, errNoBoard
		}
		statuses, err := askBoard(baseURL, token, keys)
		if err != nil {
			return nil, fmt.Errorf("reading the board: %w", err)
		}
		return statuses, nil
	}
}

// askBoard sends no Authorization header for an empty token, and a non-2xx error carries the board's answer.
func askBoard(baseURL, token string, keys []string) (map[string]string, error) {
	query := url.Values{"keys": {strings.Join(keys, ",")}, "fields": {"status"}}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/issues?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking for %s: %w", strings.Join(keys, ", "), err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("board returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var list issueList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decoding the answer %q: %w", raw, err)
	}
	statuses := map[string]string{}
	for _, is := range list.Issues {
		statuses[is.ID] = is.Status
	}
	return statuses, nil
}
