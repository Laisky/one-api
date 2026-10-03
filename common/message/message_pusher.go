package message

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/config"
)

type request struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	URL         string `json:"url"`
	Channel     string `json:"channel"`
	Token       string `json:"token"`
}

type response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SendMessage sends a message to the message pusher service.
func SendMessage(title string, description string, content string) error {
	if config.MessagePusherAddress == "" {
		return errors.New("message pusher address is not set")
	}
	req := request{
		Title:       title,
		Description: description,
		Content:     content,
		Token:       config.MessagePusherToken,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "marshal request")
	}

	resp, err := http.Post(config.MessagePusherAddress,
		"application/json", bytes.NewBuffer(data))
	if err != nil {
		return errors.Wrap(err, "send request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res response
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return errors.Wrap(err, "decode response")
	}

	if !res.Success {
		return errors.New(res.Message)
	}

	return nil
}
