package models

// Catalog represents the OSB service catalog
type Catalog struct {
	Services []Service `json:"services"`
}

// Service represents a service offering
type Service struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Bindable    bool          `json:"bindable"`
	Plans       []ServicePlan `json:"plans"`
	Metadata    *ServiceMetadata `json:"metadata,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	Requires    []string      `json:"requires,omitempty"`
}

// ServicePlan represents a service plan
type ServicePlan struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Free        *bool                  `json:"free,omitempty"`
	Bindable    *bool                  `json:"bindable,omitempty"`
	Schemas     *Schemas               `json:"schemas,omitempty"`
}

// ServiceMetadata contains optional metadata
type ServiceMetadata struct {
	DisplayName         string `json:"displayName,omitempty"`
	ImageURL            string `json:"imageUrl,omitempty"`
	LongDescription     string `json:"longDescription,omitempty"`
	ProviderDisplayName string `json:"providerDisplayName,omitempty"`
	DocumentationURL    string `json:"documentationUrl,omitempty"`
	SupportURL          string `json:"supportUrl,omitempty"`
}

// Schemas defines input/output schemas
type Schemas struct {
	ServiceInstance ServiceInstanceSchema `json:"service_instance,omitempty"`
	ServiceBinding  ServiceBindingSchema  `json:"service_binding,omitempty"`
}

// ServiceInstanceSchema defines schemas for instance operations
type ServiceInstanceSchema struct {
	Create *InputOutputSchema `json:"create,omitempty"`
	Update *InputOutputSchema `json:"update,omitempty"`
}

// ServiceBindingSchema defines schemas for binding operations
type ServiceBindingSchema struct {
	Create *InputOutputSchema `json:"create,omitempty"`
}

// InputOutputSchema defines input and output schemas
type InputOutputSchema struct {
	Parameters *interface{} `json:"parameters,omitempty"`
}

// GetInstanceResponse represents a get instance response
type GetInstanceResponse struct {
	ServiceID    string `json:"service_id"`
	PlanID       string `json:"plan_id"`
	DashboardURL string `json:"dashboard_url,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	StatusCode   int
}

// GetBindingResponse represents a get binding response
type GetBindingResponse struct {
	Credentials     map[string]interface{} `json:"credentials"`
	SyslogDrainURL  string                 `json:"syslog_drain_url,omitempty"`
	RouteServiceURL string                 `json:"route_service_url,omitempty"`
	VolumeMounts    []interface{}          `json:"volume_mounts,omitempty"`
	StatusCode      int
}