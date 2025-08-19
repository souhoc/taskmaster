package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type Webhook struct {
	Url string
}

func (w Webhook) Send(content string) error {
	if len(content) > 2000 {
		return errors.New("content is more than 2000 characters")
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(WebhookBody{Content: content}); err != nil {
		return fmt.Errorf("failed to encode webhook body: %v\n", err)
	}

	resp, err := http.Post(w.Url, "application/json", &body)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("failed to send the webhook: %s", resp.Status)
	}

	return nil
}

type WebhookBody struct {
	// Username string `json:"username"`
	Content string `json:"content"`
}
