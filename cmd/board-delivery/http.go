package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mitstack-eu/plain-delivery/internal/boardhttp"
)

type deliveryRecord struct {
	Delivery string   `json:"delivery"`
	Keys     []string `json:"keys"`
}

func postDelivered(baseURL, token, delivery string, keys []string) error {
	body, err := json.Marshal(deliveryRecord{Delivery: delivery, Keys: keys})
	if err != nil {
		return fmt.Errorf("encoding delivered payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/delivered", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = boardhttp.Send(req, token, "posting delivery")
	return err
}
