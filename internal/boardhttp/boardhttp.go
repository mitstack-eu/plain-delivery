// Package boardhttp sends this module's requests to the board, the one place their token and error rules live.
package boardhttp

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// timeout bounds a call to the board, so a board that never answers fails the job instead of holding it to the CI limit.
const timeout = 30 * time.Second

var client = &http.Client{Timeout: timeout}

// Send sends req with the token as a bearer token, none for an empty token, and returns the answer's body, or an error naming the action or carrying the board's status and answer.
func Send(req *http.Request, token, action string) ([]byte, error) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the board's answer: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("board returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}
