package overseer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/hectorgimenez/koolo/internal/config"
)

type OverseerApi struct {
	baseURL string
}

func NewOverseerApi(baseURL string) *OverseerApi {
	api := &OverseerApi{
		baseURL: baseURL,
	}
	return api
}

// CreateApiRecord is a generic function to create records for different API routes
func (api *OverseerApi) CreateApiRecord(collection string, fields map[string]string, jsonFields map[string]interface{}, fileField, fileName string, fileContent []byte) error {
	url := fmt.Sprintf("%s/api/collections/%s/records", api.baseURL, collection)

	// Create a new multipart writer
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Helper function to write a field and handle errors
	writeField := func(fieldName, value string) error {
		if err := writer.WriteField(fieldName, value); err != nil {
			return fmt.Errorf("error writing %s field: %w", fieldName, err)
		}
		return nil
	}

	// Add JSON fields
	for key, value := range jsonFields {
		jsonValue, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("error marshaling %s: %w", key, err)
		}
		if err := writeField(key, string(jsonValue)); err != nil {
			return err
		}
	}

	// Add string fields
	for key, value := range fields {
		if err := writeField(key, value); err != nil {
			return err
		}
	}

	// Add the file if provided
	if fileField != "" && fileContent != nil {
		part, err := writer.CreateFormFile(fileField, fileName)
		if err != nil {
			return fmt.Errorf("error creating form file: %w", err)
		}
		_, err = part.Write(fileContent)
		if err != nil {
			return fmt.Errorf("error writing screenshot: %v", err)
		}
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return fmt.Errorf("error closing multipart writer: %w", err)
	}

	// Create and send the request
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (api *OverseerApi) PostEvent(name, supervisor string, fieldValues map[string]interface{}) error {
	apiId := config.Characters[supervisor].Overseer.ApiSupervisorId

	if apiId == "" {
		return fmt.Errorf("API id not set")
	}

	fields := map[string]string{
		"name":       name,
		"supervisor": apiId,
	}
	jsonFields := map[string]interface{}{
		"raw": fieldValues,
	}
	return api.CreateApiRecord("events", fields, jsonFields, "", "", nil)
}

func (api *OverseerApi) PostError(err, supervisor string, screenshot []byte) error {
	apiId := config.Characters[supervisor].Overseer.ApiSupervisorId

	if apiId == "" {
		return fmt.Errorf("API id not set")
	}

	fields := map[string]string{
		"error":      err,
		"supervisor": apiId,
	}

	return api.CreateApiRecord("errors", fields, nil, "screenshot", "screenshot", screenshot)
}

func (api *OverseerApi) WebhookStopped() {
	url := fmt.Sprintf("%s/webhook/stopped", api.baseURL)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}
