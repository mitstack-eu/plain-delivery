package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitstack-eu/plain-delivery/internal/boardhttp"
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

func askBoard(baseURL, token string, keys []string) (map[string]string, error) {
	query := url.Values{"keys": {strings.Join(keys, ",")}, "fields": {"status"}}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/issues?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	raw, err := boardhttp.Send(req, token, "asking for "+strings.Join(keys, ", "))
	if err != nil {
		return nil, err
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
