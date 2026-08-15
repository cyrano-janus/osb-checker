package test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/example/osb-checker/test/models"
)

// NewOSBClient creates a new OSB API client
func NewOSBClient(baseURL, username, password, apiVersion string) *OSBClient {
	return &OSBClient{
		BaseURL:    baseURL,
		Username:   username,
		Password:   password,
		APIVersion: apiVersion,
	}
}

// GetCatalog fetches the service catalog
func (c *OSBClient) GetCatalog() (*models.Catalog, error) {
	url := fmt.Sprintf("%s/v2/catalog", c.BaseURL)
	req, err := c.newRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	var catalog models.Catalog
	if err := json.Unmarshal(resp, &catalog); err != nil {
		return nil, fmt.Errorf("failed to unmarshal catalog: %w", err)
	}

	return &catalog, nil
}

// ProvisionInstance provisions a service instance
func (c *OSBClient) ProvisionInstance(instanceID, serviceID, planID string, acceptsIncomplete bool, params map[string]interface{}) (*ProvisionResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s", c.BaseURL, instanceID)
	
	reqBody := map[string]interface{}{
		"service_id":        serviceID,
		"plan_id":           planID,
		"context": map[string]interface{}{
			"platform": "osb-checker",
		},
		"accepts_incomplete": acceptsIncomplete,
	}
	
	if params != nil {
		reqBody["parameters"] = params
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest("PUT", url, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var provisionResp ProvisionResponse
	if err := json.Unmarshal(resp.Body, &provisionResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal provision response: %w", err)
	}

	provisionResp.StatusCode = resp.StatusCode
	return &provisionResp, nil
}

// DeprovisionInstance deprovisions a service instance
func (c *OSBClient) DeprovisionInstance(instanceID, serviceID, planID string) (*DeprovisionResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s?service_id=%s&plan_id=%s", 
		c.BaseURL, instanceID, serviceID, planID)

	req, err := c.newRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var deprovResp DeprovisionResponse
	if err := json.Unmarshal(resp.Body, &deprovResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deprovision response: %w", err)
	}

	deprovResp.StatusCode = resp.StatusCode
	return &deprovResp, nil
}

// BindInstance creates a binding
func (c *OSBClient) BindInstance(instanceID, bindingID, serviceID, planID, appGUID string) (*BindResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s/service_bindings/%s", 
		c.BaseURL, instanceID, bindingID)

	reqBody := map[string]interface{}{
		"service_id": serviceID,
		"plan_id":    planID,
		"app_guid":   appGUID,
		"context": map[string]interface{}{
			"platform": "osb-checker",
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest("PUT", url, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var bindResp BindResponse
	if err := json.Unmarshal(resp.Body, &bindResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bind response: %w", err)
	}

	bindResp.StatusCode = resp.StatusCode
	return &bindResp, nil
}

// UnbindInstance removes a binding
func (c *OSBClient) UnbindInstance(instanceID, bindingID, serviceID, planID string) (*UnbindResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s/service_bindings/%s?service_id=%s&plan_id=%s",
		c.BaseURL, instanceID, bindingID, serviceID, planID)

	req, err := c.newRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var unbindResp UnbindResponse
	if err := json.Unmarshal(resp.Body, &unbindResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal unbind response: %w", err)
	}

	unbindResp.StatusCode = resp.StatusCode
	return &unbindResp, nil
}

// GetInstance fetches instance details
func (c *OSBClient) GetInstance(instanceID string) (*models.GetInstanceResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s", c.BaseURL, instanceID)

	req, err := c.newRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var instanceResp models.GetInstanceResponse
	if err := json.Unmarshal(resp.Body, &instanceResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal get instance response: %w", err)
	}

	instanceResp.StatusCode = resp.StatusCode
	return &instanceResp, nil
}

// GetBinding fetches binding details
func (c *OSBClient) GetBinding(instanceID, bindingID string) (*models.GetBindingResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s/service_bindings/%s",
		c.BaseURL, instanceID, bindingID)

	req, err := c.newRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var bindingResp models.GetBindingResponse
	if err := json.Unmarshal(resp.Body, &bindingResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal get binding response: %w", err)
	}

	bindingResp.StatusCode = resp.StatusCode
	return &bindingResp, nil
}

// UpdateInstance updates a service instance
func (c *OSBClient) UpdateInstance(instanceID, serviceID, planID string, params map[string]interface{}) (*UpdateResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s", c.BaseURL, instanceID)

	reqBody := map[string]interface{}{
		"service_id": serviceID,
		"plan_id":    planID,
		"context": map[string]interface{}{
			"platform": "osb-checker",
		},
		"previous_values": map[string]interface{}{
			"plan_id": planID,
		},
	}

	if params != nil {
		reqBody["parameters"] = params
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest("PATCH", url, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var updateResp UpdateResponse
	if err := json.Unmarshal(resp.Body, &updateResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal update response: %w", err)
	}

	updateResp.StatusCode = resp.StatusCode
	return &updateResp, nil
}

// GetLastOperation gets the last operation status
func (c *OSBClient) GetLastOperation(instanceID, operation string) (*LastOperationResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s/last_operation", c.BaseURL, instanceID)
	if operation != "" {
		url += "?operation=" + operation
	}

	req, err := c.newRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithStatus(req)
	if err != nil {
		return nil, err
	}

	var opResp LastOperationResponse
	if err := json.Unmarshal(resp.Body, &opResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal last operation response: %w", err)
	}

	opResp.StatusCode = resp.StatusCode
	return &opResp, nil
}

// HTTP helpers

func (c *OSBClient) newRequest(method, url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Broker-API-Version", c.APIVersion)
	
	if c.Username != "" && c.Password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
		req.Header.Set("Authorization", "Basic "+auth)
	}

	return req, nil
}

func (c *OSBClient) doRequest(req *http.Request) ([]byte, error) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *OSBClient) doRequestWithStatus(req *http.Request) (*ResponseWithStatus, error) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &ResponseWithStatus{
		Body:       body,
		StatusCode: resp.StatusCode,
	}, nil
}

// Response types

type ProvisionResponse struct {
	DashboardURL string `json:"dashboard_url,omitempty"`
	Operation    string `json:"operation,omitempty"`
	StatusCode   int
}

type DeprovisionResponse struct {
	Operation  string `json:"operation,omitempty"`
	StatusCode int
}

type BindResponse struct {
	Credentials     map[string]interface{} `json:"credentials"`
	SyslogDrainURL  string                 `json:"syslog_drain_url,omitempty"`
	RouteServiceURL string                 `json:"route_service_url,omitempty"`
	VolumeMounts    []interface{}          `json:"volume_mounts,omitempty"`
	Operation       string                 `json:"operation,omitempty"`
	StatusCode      int
}

type UnbindResponse struct {
	Operation  string `json:"operation,omitempty"`
	StatusCode int
}

type UpdateResponse struct {
	Operation  string `json:"operation,omitempty"`
	StatusCode int
}

type LastOperationResponse struct {
	State       string `json:"state"`
	Description string `json:"description"`
	Operation   string `json:"operation,omitempty"`
	StatusCode  int
}

type ResponseWithStatus struct {
	Body       []byte
	StatusCode int
}