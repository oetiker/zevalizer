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
	// "github.com/goccy/go-yaml"
)

type Client struct {
	config    *config.Config
	http      *http.Client
	chunkDays int // maximum days per request
}

func NewClient(config *config.Config) *Client {
	return &Client{
		config:    config,
		http:      &http.Client{},
		chunkDays: 30, // default to 5-day chunks
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
	fmt.Printf("Fetching sensors from: %s\n", path)

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
	// yamlData, err := yaml.Marshal(sensors)
	// if err != nil {
	// 	fmt.Printf("error marshaling to yaml: %v\n", err)
	// 	return nil, err
	// }
	// fmt.Println(string(yamlData))

	return sensors, nil
}

func (c *Client) GetSensorData(smId string, sensorID string, from, to time.Time) ([]models.SensorData, error) {
	var allData []models.SensorData

	// Calculate number of chunks needed
	totalDays := int(to.Sub(from).Hours()/24) + 1
	numChunks := (totalDays + c.chunkDays - 1) / c.chunkDays // Round up

	// Process each chunk
	chunkStart := from
	for chunk := 0; chunk < numChunks; chunk++ {
		// Calculate chunk end
		chunkEnd := chunkStart.Add(time.Duration(c.chunkDays) * 24 * time.Hour)
		if chunkEnd.After(to) {
			chunkEnd = to
		}

		// Build request for this chunk
		path := fmt.Sprintf("/v1/data/sensor/%s/range", sensorID)
		fromStr := chunkStart.UTC().Format("2006-01-02T15:04:05.000Z")
		toStr := chunkEnd.UTC().Format("2006-01-02T15:04:05.000Z")
		query := fmt.Sprintf("?from=%s&to=%s&interval=900", fromStr, toStr)
		fullPath := path + query
		fmt.Printf("Fetching sensor data from: %s\n", fullPath)

		req, err := c.createRequest("GET", fullPath)
		if err != nil {
			return nil, fmt.Errorf("creating request: %v", err)
		}

		// Add detailed headers
		req.Header.Add("Accept", "application/json")
		req.Header.Add("User-Agent", "zevalizer/1.0")

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

		var chunkData []models.SensorData
		if err := json.Unmarshal(body, &chunkData); err != nil {
			return nil, fmt.Errorf("decoding response: %v\nFull response: %s", err, string(body))
		}

		allData = append(allData, chunkData...)
		chunkStart = chunkEnd
	}

	return allData, nil
}

func (c *Client) GetZevData(smId string, from, to time.Time) ([]models.ZevData, error) {
	var allData []models.ZevData

	// Calculate number of chunks needed
	totalDays := int(to.Sub(from).Hours()/24) + 1
	numChunks := (totalDays + c.chunkDays - 1) / c.chunkDays // Round up

	// Process each chunk
	chunkStart := from
	fmt.Printf("Total days: %d, numChunks: %d\n", totalDays, numChunks)
	for chunk := 0; chunk < numChunks; chunk++ {
		// Calculate chunk end
		chunkEnd := chunkStart.Add(time.Duration(c.chunkDays) * 24 * time.Hour)
		if chunkEnd.After(to) {
			chunkEnd = to
		}

		path := fmt.Sprintf("/v1/data/zev/%s", smId)
		fromStr := chunkStart.UTC().Format("2006-01-02T15:04:05.000Z")
		toStr := chunkEnd.UTC().Format("2006-01-02T15:04:05.000Z")
		query := fmt.Sprintf("?from=%s&to=%s", fromStr, toStr)
		fullPath := path + query

		req, err := c.createRequest("GET", fullPath)
		if err != nil {
			return nil, fmt.Errorf("creating request: %v", err)
		}
		fmt.Printf("Fetching zev data from: %s\n", fullPath)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("making request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
		}

		var chunkData []models.ZevData
		if err := json.NewDecoder(resp.Body).Decode(&chunkData); err != nil {
			return nil, fmt.Errorf("decoding response: %v", err)
		}

		allData = append(allData, chunkData...)
		chunkStart = chunkEnd
	}

	return allData, nil
}
