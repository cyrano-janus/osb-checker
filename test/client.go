package test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cyrano-janus/osb-checker/test/config"
	"github.com/cyrano-janus/osb-checker/test/models"
)

// NewOSBClient creates a new OSB API client.
//
// Der HTTP-Client wird einmal gebaut und traegt Timeout und TLS-Material. Ohne
// eigenen Transport pruefte der Checker nur gegen die System-Roots und wartete
// unbegrenzt - gegen einen Broker mit eigener CA scheiterte damit jeder
// Request, und die Negativtests werteten das als Erfolg, weil sie jeden
// Transportfehler als "hat abgelehnt" lesen.
func NewOSBClient(cfg *config.Config) (*OSBClient, error) {
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.Insecure,
	}
	if cfg.CACert != "" {
		pem, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("read ca_cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_cert %s contains no usable certificate", cfg.CACert)
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.ClientCert != "" {
		pair, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{pair}
	}

	transport := &http.Transport{TLSClientConfig: tlsCfg}
	if cfg.Resolve != "" {
		dial, err := resolveDialer(cfg.Resolve)
		if err != nil {
			return nil, err
		}
		transport.DialContext = dial
	}

	return &OSBClient{
		BaseURL:    cfg.BrokerURL,
		Username:   cfg.Username,
		Password:   cfg.Password,
		APIVersion: cfg.APIVersion,
		http: &http.Client{
			Timeout:   time.Duration(cfg.TimeoutSeconds) * time.Second,
			Transport: transport,
		},
		acceptsAsync:       cfg.AcceptsAsync,
		pollTimeoutSeconds: cfg.PollTimeoutSeconds,
	}, nil
}

// resolveDialer baut einen Dialer, der genau einen Hostnamen auf eine andere
// Adresse umlenkt - wie curls --resolve. Die TLS-Pruefung laeuft weiter gegen
// den urspruenglichen Namen, nur die Verbindung geht woanders hin.
func resolveDialer(spec string) (func(context.Context, string, string) (net.Conn, error), error) {
	parts := strings.Split(spec, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("resolve %q: expected host:port:address", spec)
	}
	from := net.JoinHostPort(parts[0], parts[1])
	to := net.JoinHostPort(parts[2], parts[1])
	base := &net.Dialer{Timeout: 10 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == from {
			addr = to
		}
		return base.DialContext(ctx, network, addr)
	}, nil
}

// AcceptsAsync reports whether the checker announces async support.
func (c *OSBClient) AcceptsAsync() bool { return c.acceptsAsync }

// asyncQuery liefert den Query-Parameter, den die Spezifikation verlangt.
//
// accepts_incomplete ist ein QUERY-Parameter, kein Body-Feld. Im Body gesetzt
// sieht ein spec-konformer Broker ihn als nicht vorhanden - der Checker
// behauptete also Async-Faehigkeit und bekam sie nie zugestanden.
func (c *OSBClient) asyncQuery(sep string) string {
	if !c.acceptsAsync {
		return ""
	}
	return sep + "accepts_incomplete=true"
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
func (c *OSBClient) ProvisionInstance(instanceID, serviceID, planID string, _ bool, params map[string]interface{}) (*ProvisionResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s%s", c.BaseURL, instanceID, c.asyncQuery("?"))

	reqBody := map[string]interface{}{
		"service_id": serviceID,
		"plan_id":    planID,
		"context": map[string]interface{}{
			"platform": "osb-checker",
		},
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
	url := fmt.Sprintf("%s/v2/service_instances/%s?service_id=%s&plan_id=%s"+c.asyncQuery("&"),
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
	url := fmt.Sprintf("%s/v2/service_instances/%s/service_bindings/%s%s",
		c.BaseURL, instanceID, bindingID, c.asyncQuery("?"))

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

// UpdateInstanceParameters schickt ein PATCH, das nur Parameter traegt - ohne
// plan_id. Genau die Form, die `cf update-service -c '{...}'` erzeugt.
//
// UpdateInstance schickt plan_id immer mit; damit laesst sich nicht pruefen,
// ob ein Broker es faelschlich verlangt. Laut OSB 2.17 ist das Feld im PATCH
// optional.
func (c *OSBClient) UpdateInstanceParameters(instanceID, serviceID string, params map[string]interface{}) (*UpdateResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s%s", c.BaseURL, instanceID, c.asyncQuery("?"))

	body, err := json.Marshal(map[string]interface{}{
		"service_id": serviceID,
		"parameters": params,
		"context":    map[string]interface{}{"platform": "osb-checker"},
	})
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
	url := fmt.Sprintf("%s/v2/service_instances/%s%s", c.BaseURL, instanceID, c.asyncQuery("?"))

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
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *OSBClient) doRequestWithStatus(req *http.Request) (*ResponseWithStatus, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
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

// WaitForOperation pollt last_operation, bis der Vorgang abgeschlossen ist.
//
// Ein Broker, der mit 202 antwortet, ist noch nicht fertig - wer danach sofort
// bindet, bindet gegen Zugangsdaten, die es noch nicht gibt. Genau dafuer
// existiert last_operation, und ein Checker, der es nur einmal abfragt, prueft
// die Zusage nicht.
//
// bindingID darf leer sein; dann gilt die Abfrage der Instanz.
func (c *OSBClient) WaitForOperation(instanceID, bindingID, operation string) (string, error) {
	deadline := time.Now().Add(time.Duration(c.pollTimeoutSeconds) * time.Second)
	// Eine Sekunde zu Beginn, danach langsam laenger: ein frisch angelegter
	// Service braucht Minuten, und ein Poll je Sekunde ueber Minuten belastet
	// den Broker ohne Erkenntnisgewinn.
	wait := time.Second
	for {
		resp, err := c.lastOperation(instanceID, bindingID, operation)
		if err != nil {
			return "", err
		}
		switch {
		case resp.StatusCode == http.StatusGone:
			// Beim Deprovision ist 410 das erwartete Ende.
			return "gone", nil
		case resp.StatusCode != http.StatusOK:
			return "", fmt.Errorf("last_operation -> HTTP %d", resp.StatusCode)
		}
		switch resp.State {
		case "succeeded", "failed":
			return resp.State, nil
		case "in progress", "":
			// weiter warten
		default:
			return "", fmt.Errorf("last_operation: unknown state %q", resp.State)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("last_operation still %q after %ds", resp.State, c.pollTimeoutSeconds)
		}
		time.Sleep(wait)
		if wait < 10*time.Second {
			wait += time.Second
		}
	}
}

// lastOperation fragt den Zustand einer Instanz oder eines Bindings ab.
func (c *OSBClient) lastOperation(instanceID, bindingID, operation string) (*LastOperationResponse, error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s", c.BaseURL, instanceID)
	if bindingID != "" {
		url += "/service_bindings/" + bindingID
	}
	url += "/last_operation"
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
	// Ein leerer Koerper ist bei 410 der Normalfall; das darf den Aufrufer
	// nicht mit einem Parse-Fehler behelligen.
	if len(resp.Body) > 0 {
		_ = json.Unmarshal(resp.Body, &opResp)
	}
	opResp.StatusCode = resp.StatusCode
	return &opResp, nil
}
