package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"zevalizer/internal/config"
	"zevalizer/internal/models"
)

type Client struct {
	config    *config.Config
	http      *http.Client
	chunkDays int // maximum days per request
}

func (c *Client) debugf(format string, args ...interface{}) {
	if c.config.Debug {
		fmt.Printf("DEBUG: "+format+"\n", args...)
	}
}

func NewClient(config *config.Config) *Client {
	return &Client{
		config: config,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		chunkDays: 30,
	}
}

func (c *Client) createRequest(method, path string) (*http.Request, error) {
	req, err := http.NewRequest(method, c.config.API.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}

	// Add basic auth
	auth := base64.StdEncoding.EncodeToString([]byte(c.config.API.Username + ":" + c.config.API.Password))
	req.Header.Add("Authorization", "Basic "+auth)

	return req, nil
}

// timeChunk represents a chunk of time for date range requests
type timeChunk struct {
	Start time.Time
	End   time.Time
}

// calculateChunks splits a date range into smaller chunks of up to chunkDays days each
func (c *Client) calculateChunks(from, to time.Time) []timeChunk {
	totalDays := int(to.Sub(from).Hours()/24) + 1
	numChunks := (totalDays + c.chunkDays - 1) / c.chunkDays

	chunks := make([]timeChunk, 0, numChunks)
	chunkStart := from

	for i := 0; i < numChunks; i++ {
		chunkEnd := chunkStart.Add(time.Duration(c.chunkDays) * 24 * time.Hour)
		if chunkEnd.After(to) {
			chunkEnd = to
		}
		chunks = append(chunks, timeChunk{Start: chunkStart, End: chunkEnd})
		chunkStart = chunkEnd
	}

	return chunks
}

// fetchChunkedData performs an HTTP GET request and returns the response body.
// The caller is responsible for unmarshaling the JSON response.
func (c *Client) fetchChunkedData(path string) ([]byte, error) {
	req, err := c.createRequest("GET", path)
	if err != nil {
		return nil, fmt.Errorf("creating request: %v", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) TestConnection() error {
	req, err := c.createRequest("GET", "/v1/overview")
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetUsers() ([]models.User, error) {
	req, err := c.createRequest("GET", "/v1/users")
	if err != nil {
		return nil, fmt.Errorf("creating request: %v", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var users []models.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("decoding response: %v", err)
	}

	return users, nil
}

func (c *Client) GetSensors(smID string) ([]models.Sensor, error) {
	path := fmt.Sprintf("/v1/info/sensors/%s", smID)
	c.debugf("Fetching sensors from: %s", path)

	req, err := c.createRequest("GET", path)
	if err != nil {
		return nil, fmt.Errorf("creating request: %v", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var sensors []models.Sensor
	if err := json.Unmarshal(body, &sensors); err != nil {
		return nil, fmt.Errorf("decoding response: %v\nResponse body: %s", err, string(body))
	}

	return sensors, nil
}

func (c *Client) GetSensorData(smId string, sensorID string, from, to time.Time) ([]models.SensorData, error) {
	var allData []models.SensorData
	chunks := c.calculateChunks(from, to)

	for _, chunk := range chunks {
		fromStr := chunk.Start.UTC().Format("2006-01-02T15:04:05.000Z")
		toStr := chunk.End.UTC().Format("2006-01-02T15:04:05.000Z")
		path := fmt.Sprintf("/v1/data/sensor/%s/range?from=%s&to=%s&interval=900", sensorID, fromStr, toStr)
		c.debugf("Fetching sensor data from: %s", path)

		body, err := c.fetchChunkedData(path)
		if err != nil {
			return nil, err
		}

		var chunkData []models.SensorData
		if err := json.Unmarshal(body, &chunkData); err != nil {
			return nil, fmt.Errorf("decoding response: %v\nFull response: %s", err, string(body))
		}

		allData = append(allData, chunkData...)
	}

	return allData, nil
}

func (c *Client) GetZevData(smId string, from, to time.Time) ([]models.ZevData, error) {
	var allData []models.ZevData
	chunks := c.calculateChunks(from, to)
	c.debugf("Total days: %d, numChunks: %d", len(chunks)*c.chunkDays, len(chunks))

	for _, chunk := range chunks {
		fromStr := chunk.Start.UTC().Format("2006-01-02T15:04:05.000Z")
		toStr := chunk.End.UTC().Format("2006-01-02T15:04:05.000Z")
		path := fmt.Sprintf("/v1/data/zev/%s?from=%s&to=%s", smId, fromStr, toStr)
		c.debugf("Fetching zev data from: %s", path)

		body, err := c.fetchChunkedData(path)
		if err != nil {
			return nil, err
		}

		var chunkData []models.ZevData
		if err := json.Unmarshal(body, &chunkData); err != nil {
			return nil, fmt.Errorf("decoding response: %v", err)
		}

		allData = append(allData, chunkData...)
	}

	return allData, nil
}
