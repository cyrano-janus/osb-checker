package test

import "github.com/example/osb-checker/test/models"

// TestResults contains the results of running the test suite
type TestResults struct {
	Total     int
	Passed    int
	Failed    int
	Skipped   int
	Successes []TestResult
	Failures  []TestFailure
}

// TestResult represents a successful test result
type TestResult struct {
	TestName string
	Category string
}

// TestFailure represents a failed test
type TestFailure struct {
	TestName string
	Category string
	Error    string
	Endpoint string
	Method   string
}

// TestSuite represents the complete test suite
type TestSuite struct {
	config  *Config
	verbose bool
	client  *OSBClient
}

// Config holds test configuration
type Config struct {
	BrokerURL     string
	Username      string
	Password      string
	APIVersion    string
	AcceptsAsync  bool
	TestCatalog   bool
	TestProvision bool
	TestBind      bool
	TestUpdate    bool
	TestFetch     bool
}

// OSBClient is the HTTP client for OSB API
type OSBClient struct {
	BaseURL    string
	Username   string
	Password   string
	APIVersion string
}

// Test state
type TestState struct {
	InstanceID string
	BindingID  string
	ServiceID  string
	PlanID     string
	Catalog    *models.Catalog
}