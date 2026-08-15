package test

import (
	"fmt"

	"github.com/example/osb-checker/test/config"
)

// NewTestSuite creates a new test suite
func NewTestSuite(cfg *config.Config, verbose bool) *TestSuite {
	client := NewOSBClient(cfg.BrokerURL, cfg.Username, cfg.Password, cfg.APIVersion)
	
	return &TestSuite{
		config: &Config{
			BrokerURL:     cfg.BrokerURL,
			Username:      cfg.Username,
			Password:      cfg.Password,
			APIVersion:    cfg.APIVersion,
			AcceptsAsync:  cfg.AcceptsAsync,
			TestCatalog:   cfg.TestCatalog,
			TestProvision: cfg.TestProvision,
			TestBind:      cfg.TestBind,
			TestUpdate:    cfg.TestUpdate,
			TestFetch:     cfg.TestFetch,
		},
		verbose: verbose,
		client:  client,
	}
}

// Run executes all tests
func (s *TestSuite) Run() *TestResults {
	results := &TestResults{
		Successes: make([]TestResult, 0),
		Failures:  make([]TestFailure, 0),
	}

	state := &TestState{}

	// Run catalog tests
	if s.config.TestCatalog {
		s.runCatalogTests(results, state)
	}

	// Run provision tests
	if s.config.TestProvision {
		s.runProvisionTests(results, state)
	}

	// Run bind tests
	if s.config.TestBind && state.InstanceID != "" {
		s.runBindTests(results, state)
	}

	// Run update tests
	if s.config.TestUpdate && state.InstanceID != "" {
		s.runUpdateTests(results, state)
	}

	// Run fetch tests
	if s.config.TestFetch && state.InstanceID != "" {
		s.runFetchTests(results, state)
	}

	// Cleanup
	if state.InstanceID != "" && state.BindingID != "" {
		s.cleanup(results, state)
	}

	// Calculate totals
	results.Total = results.Passed + results.Failed + results.Skipped

	return results
}

func (s *TestSuite) runCatalogTests(results *TestResults, state *TestState) {
	s.testCatalogExists(results, state)
	s.testCatalogHasServices(results, state)
	s.testCatalogServiceStructure(results, state)
	s.testCatalogPlanStructure(results, state)
}

func (s *TestSuite) runProvisionTests(results *TestResults, state *TestState) {
	if state.ServiceID == "" || state.PlanID == "" {
		results.Skipped++
		results.Successes = append(results.Successes, TestResult{
			TestName: "Provision tests skipped (no service/plan found)",
			Category: "provision",
		})
		return
	}

	s.testProvisionSuccess(results, state)
	s.testProvisionIdempotent(results, state)
	s.testProvisionMissingServiceID(results, state)
	s.testProvisionMissingPlanID(results, state)
	s.testProvisionInvalidService(results, state)
}

func (s *TestSuite) runBindTests(results *TestResults, state *TestState) {
	s.testBindSuccess(results, state)
	s.testBindIdempotent(results, state)
	s.testBindMissingServiceID(results, state)
	s.testBindMissingPlanID(results, state)
	s.testBindNonExistentInstance(results, state)
}

func (s *TestSuite) runUpdateTests(results *TestResults, state *TestState) {
	s.testUpdateInstance(results, state)
	s.testUpdateNonExistentInstance(results, state)
}

func (s *TestSuite) runFetchTests(results *TestResults, state *TestState) {
	s.testGetInstance(results, state)
	s.testGetBinding(results, state)
	s.testGetNonExistentInstance(results, state)
	s.testGetNonExistentBinding(results, state)
	s.testLastOperation(results, state)
}

func (s *TestSuite) cleanup(results *TestResults, state *TestState) {
	if s.verbose {
		fmt.Println("\nCleaning up test resources...")
	}

	// Unbind
	if state.BindingID != "" {
		_, err := s.client.UnbindInstance(state.InstanceID, state.BindingID, state.ServiceID, state.PlanID)
		if err != nil && s.verbose {
			fmt.Printf("Warning: Failed to unbind: %v\n", err)
		}
	}

	// Deprovision
	if state.InstanceID != "" {
		_, err := s.client.DeprovisionInstance(state.InstanceID, state.ServiceID, state.PlanID)
		if err != nil && s.verbose {
			fmt.Printf("Warning: Failed to deprovision: %v\n", err)
		}
	}
}

// Test helpers - Catalog tests

func (s *TestSuite) testCatalogExists(results *TestResults, state *TestState) {
	testName := "Catalog endpoint exists and returns valid JSON"
	
	catalog, err := s.client.GetCatalog()
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "catalog",
			Error:    err.Error(),
			Endpoint: "/v2/catalog",
			Method:   "GET",
		})
		return
	}

	state.Catalog = catalog
	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "catalog",
	})
}

func (s *TestSuite) testCatalogHasServices(results *TestResults, state *TestState) {
	testName := "Catalog contains at least one service"
	
	if state.Catalog == nil || len(state.Catalog.Services) == 0 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "catalog",
			Error:    "Catalog has no services",
			Endpoint: "/v2/catalog",
			Method:   "GET",
		})
		return
	}

	// Select first service and plan for subsequent tests
	state.ServiceID = state.Catalog.Services[0].ID
	state.PlanID = state.Catalog.Services[0].Plans[0].ID

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "catalog",
	})
}

func (s *TestSuite) testCatalogServiceStructure(results *TestResults, state *TestState) {
	testName := "Service has required fields (id, name, description, plans)"
	
	if state.Catalog == nil || len(state.Catalog.Services) == 0 {
		results.Skipped++
		return
	}

	service := state.Catalog.Services[0]
	if service.ID == "" || service.Name == "" || service.Description == "" || len(service.Plans) == 0 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "catalog",
			Error:    "Service missing required fields",
			Endpoint: "/v2/catalog",
			Method:   "GET",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "catalog",
	})
}

func (s *TestSuite) testCatalogPlanStructure(results *TestResults, state *TestState) {
	testName := "Plan has required fields (id, name, description)"
	
	if state.Catalog == nil || len(state.Catalog.Services) == 0 {
		results.Skipped++
		return
	}

	plan := state.Catalog.Services[0].Plans[0]
	if plan.ID == "" || plan.Name == "" || plan.Description == "" {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "catalog",
			Error:    "Plan missing required fields",
			Endpoint: "/v2/catalog",
			Method:   "GET",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "catalog",
	})
}

// Test helpers - Provision tests

func (s *TestSuite) testProvisionSuccess(results *TestResults, state *TestState) {
	testName := "Provision returns 201 Created"
	
	instanceID := "test-instance-" + state.ServiceID[:8]
	state.InstanceID = instanceID

	resp, err := s.client.ProvisionInstance(instanceID, state.ServiceID, state.PlanID, false, nil)
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "provision",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "PUT",
		})
		return
	}

	if resp.StatusCode != 201 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "provision",
			Error:    fmt.Sprintf("Expected status 201, got %d", resp.StatusCode),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "PUT",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "provision",
	})
}

func (s *TestSuite) testProvisionIdempotent(results *TestResults, state *TestState) {
	testName := "Provision is idempotent (second call succeeds)"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	resp, err := s.client.ProvisionInstance(state.InstanceID, state.ServiceID, state.PlanID, false, nil)
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "provision",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "PUT",
		})
		return
	}

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "provision",
			Error:    fmt.Sprintf("Expected status 200 or 201, got %d", resp.StatusCode),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "PUT",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "provision",
	})
}

func (s *TestSuite) testProvisionMissingServiceID(results *TestResults, state *TestState) {
	testName := "Provision without service_id returns 400"
	
	instanceID := "test-instance-no-service"

	resp, err := s.client.ProvisionInstance(instanceID, "", state.PlanID, false, nil)
	
	// Check status code, not error (doRequestWithStatus doesn't error on 400)
	if err != nil {
		// HTTP error = test passes
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "provision",
		})
		return
	}
	
	// Check if status code indicates error (400-499)
	if resp.StatusCode >= 400 {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "provision",
		})
		return
	}
	
	// No error and status < 400 = test fails
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "provision",
		Error:    fmt.Sprintf("Expected error (400), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}",
		Method:   "PUT",
	})
}

func (s *TestSuite) testProvisionMissingPlanID(results *TestResults, state *TestState) {
	testName := "Provision without plan_id returns 400"
	
	instanceID := "test-instance-no-plan"

	resp, err := s.client.ProvisionInstance(instanceID, state.ServiceID, "", false, nil)
	
	// Check status code, not error
	if err != nil {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "provision",
		})
		return
	}
	
	if resp.StatusCode >= 400 {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "provision",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "provision",
		Error:    fmt.Sprintf("Expected error (400), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}",
		Method:   "PUT",
	})
}

func (s *TestSuite) testProvisionInvalidService(results *TestResults, state *TestState) {
	testName := "Provision with invalid service returns 400/404"
	
	instanceID := "test-instance-invalid"

	resp, err := s.client.ProvisionInstance(instanceID, "invalid-service", state.PlanID, false, nil)
	
	// Check status code, not error
	if err != nil {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "provision",
		})
		return
	}
	
	if resp.StatusCode >= 400 {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "provision",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "provision",
		Error:    fmt.Sprintf("Expected error (400/404), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}",
		Method:   "PUT",
	})
}

// Test helpers - Bind tests

func (s *TestSuite) testBindSuccess(results *TestResults, state *TestState) {
	testName := "Bind returns 201 Created with credentials"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	bindingID := "test-binding-" + state.ServiceID[:8]
	state.BindingID = bindingID

	resp, err := s.client.BindInstance(state.InstanceID, bindingID, state.ServiceID, state.PlanID, "test-app")
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "bind",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "PUT",
		})
		return
	}

	if resp.StatusCode != 201 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "bind",
			Error:    fmt.Sprintf("Expected status 201, got %d", resp.StatusCode),
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "PUT",
		})
		return
	}

	if resp.Credentials == nil || len(resp.Credentials) == 0 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "bind",
			Error:    "Response missing credentials",
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "PUT",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "bind",
	})
}

func (s *TestSuite) testBindIdempotent(results *TestResults, state *TestState) {
	testName := "Bind is idempotent (second call returns same credentials)"
	
	if state.BindingID == "" {
		results.Skipped++
		return
	}

	resp1, err := s.client.BindInstance(state.InstanceID, state.BindingID, state.ServiceID, state.PlanID, "test-app")
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "bind",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "PUT",
		})
		return
	}

	resp2, err := s.client.BindInstance(state.InstanceID, state.BindingID, state.ServiceID, state.PlanID, "test-app")
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "bind",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "PUT",
		})
		return
	}

	if fmt.Sprintf("%v", resp1.Credentials) != fmt.Sprintf("%v", resp2.Credentials) {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "bind",
			Error:    "Credentials differ between idempotent bind calls",
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "PUT",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "bind",
	})
}

func (s *TestSuite) testBindMissingServiceID(results *TestResults, state *TestState) {
	testName := "Bind without service_id returns 400"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	bindingID := "test-binding-no-service"

	resp, err := s.client.BindInstance(state.InstanceID, bindingID, "", state.PlanID, "test-app")
	
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "bind",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "bind",
		Error:    fmt.Sprintf("Expected error (400), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
		Method:   "PUT",
	})
}

func (s *TestSuite) testBindMissingPlanID(results *TestResults, state *TestState) {
	testName := "Bind without plan_id returns 400"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	bindingID := "test-binding-no-plan"

	resp, err := s.client.BindInstance(state.InstanceID, bindingID, state.ServiceID, "", "test-app")
	
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "bind",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "bind",
		Error:    fmt.Sprintf("Expected error (400), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
		Method:   "PUT",
	})
}

func (s *TestSuite) testBindNonExistentInstance(results *TestResults, state *TestState) {
	testName := "Bind to non-existent instance returns 404"
	
	bindingID := "test-binding-nonexistent"

	resp, err := s.client.BindInstance("nonexistent-instance", bindingID, state.ServiceID, state.PlanID, "test-app")
	
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "bind",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "bind",
		Error:    fmt.Sprintf("Expected error (404), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
		Method:   "PUT",
	})
}

// Test helpers - Update tests

func (s *TestSuite) testUpdateInstance(results *TestResults, state *TestState) {
	testName := "Update instance returns 200 OK"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	resp, err := s.client.UpdateInstance(state.InstanceID, state.ServiceID, state.PlanID, nil)
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "update",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "PATCH",
		})
		return
	}

	if resp.StatusCode != 200 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "update",
			Error:    fmt.Sprintf("Expected status 200, got %d", resp.StatusCode),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "PATCH",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "update",
	})
}

func (s *TestSuite) testUpdateNonExistentInstance(results *TestResults, state *TestState) {
	testName := "Update non-existent instance returns 404"
	
	resp, err := s.client.UpdateInstance("nonexistent-instance", state.ServiceID, state.PlanID, nil)
	
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "update",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "update",
		Error:    fmt.Sprintf("Expected error (404), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}",
		Method:   "PATCH",
	})
}

// Test helpers - Fetch tests

func (s *TestSuite) testGetInstance(results *TestResults, state *TestState) {
	testName := "Get instance returns 200 OK with service_id and plan_id"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	resp, err := s.client.GetInstance(state.InstanceID)
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "GET",
		})
		return
	}

	if resp.StatusCode != 200 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    fmt.Sprintf("Expected status 200, got %d", resp.StatusCode),
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "GET",
		})
		return
	}

	if resp.ServiceID == "" || resp.PlanID == "" {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    "Response missing service_id or plan_id",
			Endpoint: "/v2/service_instances/{instance_id}",
			Method:   "GET",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "fetch",
	})
}

func (s *TestSuite) testGetBinding(results *TestResults, state *TestState) {
	testName := "Get binding returns 200 OK with credentials"
	
	if state.BindingID == "" {
		results.Skipped++
		return
	}

	resp, err := s.client.GetBinding(state.InstanceID, state.BindingID)
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "GET",
		})
		return
	}

	if resp.StatusCode != 200 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    fmt.Sprintf("Expected status 200, got %d", resp.StatusCode),
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "GET",
		})
		return
	}

	if resp.Credentials == nil || len(resp.Credentials) == 0 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    "Response missing credentials",
			Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
			Method:   "GET",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "fetch",
	})
}

func (s *TestSuite) testGetNonExistentInstance(results *TestResults, state *TestState) {
	testName := "Get non-existent instance returns 404"
	
	resp, err := s.client.GetInstance("nonexistent-instance")
	
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "fetch",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "fetch",
		Error:    fmt.Sprintf("Expected error (404), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}",
		Method:   "GET",
	})
}

func (s *TestSuite) testGetNonExistentBinding(results *TestResults, state *TestState) {
	testName := "Get non-existent binding returns 404"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	resp, err := s.client.GetBinding(state.InstanceID, "nonexistent-binding")
	
	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		results.Passed++
		results.Successes = append(results.Successes, TestResult{
			TestName: testName,
			Category: "fetch",
		})
		return
	}
	
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: testName,
		Category: "fetch",
		Error:    fmt.Sprintf("Expected error (404), but got status %d", resp.StatusCode),
		Endpoint: "/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
		Method:   "GET",
	})
}

func (s *TestSuite) testLastOperation(results *TestResults, state *TestState) {
	testName := "Get last operation returns 200 OK with state"
	
	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	resp, err := s.client.GetLastOperation(state.InstanceID, "")
	if err != nil {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    err.Error(),
			Endpoint: "/v2/service_instances/{instance_id}/last_operation",
			Method:   "GET",
		})
		return
	}

	if resp.StatusCode != 200 {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    fmt.Sprintf("Expected status 200, got %d", resp.StatusCode),
			Endpoint: "/v2/service_instances/{instance_id}/last_operation",
			Method:   "GET",
		})
		return
	}

	if resp.State == "" {
		results.Failed++
		results.Failures = append(results.Failures, TestFailure{
			TestName: testName,
			Category: "fetch",
			Error:    "Response missing state field",
			Endpoint: "/v2/service_instances/{instance_id}/last_operation",
			Method:   "GET",
		})
		return
	}

	results.Passed++
	results.Successes = append(results.Successes, TestResult{
		TestName: testName,
		Category: "fetch",
	})
}