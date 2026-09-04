package test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/cyrano-janus/osb-checker/test/config"
	"github.com/cyrano-janus/osb-checker/test/models"
)

// NewTestSuite creates a new test suite.
func NewTestSuite(cfg *config.Config, verbose bool) (*TestSuite, error) {
	client, err := NewOSBClient(cfg)
	if err != nil {
		return nil, err
	}
	return &TestSuite{config: cfg, verbose: verbose, client: client}, nil
}

// runID trennt gleichzeitige Laeufe voneinander. Zwei CI-Jobs gegen denselben
// Broker duerfen sich nicht auf dieselbe Instanz setzen - vorher waren die IDs
// aus der Service-ID abgeleitet und damit fuer jeden Lauf gleich.
func runID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "fixed"
	}
	return hex.EncodeToString(b)
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

	// Aufgeraeumt wird, was entstanden ist - unabhaengig davon, wie weit der
	// Lauf gekommen ist.
	if len(state.CreatedInstances) > 0 || len(state.CreatedBindings) > 0 {
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

// cleanup raeumt jede Instanz und jedes Binding ab, das im Lauf wirklich
// entstanden ist.
//
// Zwei Dinge waren hier falsch: aufgeraeumt wurde nur, wenn sowohl Instanz als
// auch Binding existierten - lief der Bind-Teil nicht, blieb die Instanz
// stehen. Und ein gescheitertes Aufraeumen wurde nur bei -v gedruckt und nie
// gezaehlt, sodass ein Broker, der nicht deprovisionieren kann, "alle Tests
// bestanden" meldete.
func (s *TestSuite) cleanup(results *TestResults, state *TestState) {
	if s.verbose {
		fmt.Println("\nCleaning up test resources...")
	}

	for _, b := range state.CreatedBindings {
		if _, err := s.client.UnbindInstance(b.InstanceID, b.BindingID, state.ServiceID, state.PlanID); err != nil {
			s.recordFailure(results, "Cleanup: unbind", "cleanup", err.Error(),
				"/v2/service_instances/"+b.InstanceID+"/service_bindings/"+b.BindingID, "DELETE")
		} else {
			s.recordSuccess(results, "Cleanup: unbind "+b.BindingID, "cleanup")
		}
	}

	for _, id := range state.CreatedInstances {
		if _, err := s.client.DeprovisionInstance(id, state.ServiceID, state.PlanID); err != nil {
			s.recordFailure(results, "Cleanup: deprovision", "cleanup", err.Error(),
				"/v2/service_instances/"+id, "DELETE")
			continue
		}
		if s.client.AcceptsAsync() {
			// Ein asynchrones Deprovision ist erst fertig, wenn
			// last_operation es sagt. Wer hier nicht wartet, laesst
			// moeglicherweise eine halb abgeraeumte Instanz zurueck.
			if _, err := s.client.WaitForOperation(id, "", ""); err != nil && s.verbose {
				fmt.Printf("Warning: deprovision of %s did not settle: %v\n", id, err)
			}
		}
		s.recordSuccess(results, "Cleanup: deprovision "+id, "cleanup")
	}
}

// recordSuccess und recordFailure buchen ein Ergebnis.
func (s *TestSuite) recordSuccess(results *TestResults, name, category string) {
	results.Passed++
	results.Successes = append(results.Successes, TestResult{TestName: name, Category: category})
}

func (s *TestSuite) recordFailure(results *TestResults, name, category, msg, endpoint, method string) {
	results.Failed++
	results.Failures = append(results.Failures, TestFailure{
		TestName: name, Category: category, Error: msg, Endpoint: endpoint, Method: method,
	})
}

// isClientError prueft auf 4xx.
//
// Vorher stand hier >= 400, der Kommentar sprach aber von 400-499: ein Broker,
// der bei einer fehlenden service_id mit 500 abstuerzt, bestand den Test.
func isClientError(status int) bool { return status >= 400 && status < 500 }

// pickService waehlt Service und Plan.
//
// Vorgabe hat Vorrang und muss aufgehen; ein unbekannter Wert ist ein
// Fehlschlag und kein stiller Rueckfall, sonst prueft ein CI-Lauf klaglos
// etwas anderes als gemeint. Ohne Vorgabe faellt die Wahl auf den ersten
// Service, der nicht in skip_services steht und mindestens einen Plan hat -
// welcher Service im Katalog vorn steht, ist keine Zusage des Brokers.
func (s *TestSuite) pickService(results *TestResults, state *TestState) {
	const name = "Service selection"
	svcs := state.Catalog.Services

	if s.config.ServiceID != "" {
		for _, svc := range svcs {
			if svc.ID != s.config.ServiceID {
				continue
			}
			plan, err := selectPlan(svc, s.config.PlanID)
			if err != nil {
				s.recordFailure(results, name, "catalog", err.Error(), "/v2/catalog", "GET")
				return
			}
			state.ServiceID, state.PlanID = svc.ID, plan
			s.recordSuccess(results, fmt.Sprintf("%s: %s / %s (configured)", name, svc.Name, plan), "catalog")
			return
		}
		s.recordFailure(results, name, "catalog",
			fmt.Sprintf("service_id %q is not in the catalog", s.config.ServiceID), "/v2/catalog", "GET")
		return
	}

	skip := make(map[string]bool, len(s.config.SkipServices))
	for _, id := range s.config.SkipServices {
		skip[id] = true
	}
	for _, svc := range svcs {
		if skip[svc.ID] {
			continue
		}
		plan, err := selectPlan(svc, "")
		if err != nil {
			continue
		}
		state.ServiceID, state.PlanID = svc.ID, plan
		s.recordSuccess(results, fmt.Sprintf("%s: %s / %s (first eligible)", name, svc.Name, plan), "catalog")
		return
	}
	s.recordFailure(results, name, "catalog", "no service with at least one plan", "/v2/catalog", "GET")
}

// selectPlan liefert den gewuenschten Plan oder den ersten des Service.
func selectPlan(svc models.Service, wanted string) (string, error) {
	if wanted != "" {
		for _, p := range svc.Plans {
			if p.ID == wanted {
				return p.ID, nil
			}
		}
		return "", fmt.Errorf("plan_id %q does not belong to service %q", wanted, svc.ID)
	}
	// Ohne diese Pruefung lief Plans[0] in einen Index-Panic, sobald ein
	// Service ohne Plaene im Katalog stand.
	if len(svc.Plans) == 0 {
		return "", fmt.Errorf("service %q offers no plan", svc.ID)
	}
	return svc.Plans[0].ID, nil
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

	s.pickService(results, state)

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
	testName := "Provision accepted (201 or 202)"

	// Die ID traegt den Praefix aus der Konfiguration und eine Zufallszahl.
	// Frueher stand hier state.ServiceID[:8] - das ergab fuer jeden Lauf
	// dieselbe ID (zwei parallele Laeufe kollidierten) und lief bei einer
	// Service-ID unter acht Zeichen in einen Panic.
	instanceID := fmt.Sprintf("%s-inst-%s", s.config.IDPrefix, runID())
	state.InstanceID = instanceID

	resp, err := s.client.ProvisionInstance(instanceID, state.ServiceID, state.PlanID, s.client.AcceptsAsync(), nil)
	if err != nil {
		s.recordFailure(results, testName, "provision", err.Error(),
			"/v2/service_instances/{instance_id}", "PUT")
		return
	}

	// Ein Broker darf synchron (201) oder asynchron (202) antworten. Nur 201
	// zu akzeptieren hiess: jeder Broker, der fuer echte Dienste die
	// vorgesehene asynchrone Antwort gibt, faellt durch.
	switch resp.StatusCode {
	case http.StatusCreated:
		state.CreatedInstances = append(state.CreatedInstances, instanceID)
	case http.StatusAccepted:
		state.CreatedInstances = append(state.CreatedInstances, instanceID)
		st, err := s.client.WaitForOperation(instanceID, "", resp.Operation)
		if err != nil {
			s.recordFailure(results, testName+" (async)", "provision", err.Error(),
				"/v2/service_instances/{instance_id}/last_operation", "GET")
			return
		}
		if st != "succeeded" {
			s.recordFailure(results, testName+" (async)", "provision",
				fmt.Sprintf("last_operation ended in %q", st),
				"/v2/service_instances/{instance_id}/last_operation", "GET")
			return
		}
	default:
		s.recordFailure(results, testName, "provision",
			fmt.Sprintf("Expected status 201 or 202, got %d", resp.StatusCode),
			"/v2/service_instances/{instance_id}", "PUT")
		return
	}

	s.recordSuccess(results, testName, "provision")
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

	instanceID := fmt.Sprintf("%s-no-svc-%s", s.config.IDPrefix, runID())

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
	if isClientError(resp.StatusCode) {
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

	instanceID := fmt.Sprintf("%s-no-plan-%s", s.config.IDPrefix, runID())

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

	if isClientError(resp.StatusCode) {
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

	instanceID := fmt.Sprintf("%s-invalid-%s", s.config.IDPrefix, runID())

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

	if isClientError(resp.StatusCode) {
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
	testName := "Bind accepted (201 or 202) with credentials"

	if state.InstanceID == "" {
		results.Skipped++
		return
	}

	bindingID := fmt.Sprintf("%s-bind-%s", s.config.IDPrefix, runID())
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

	// Wie beim Provision: 201 synchron, 202 asynchron. Bei 202 gibt es noch
	// keine Credentials - die stehen erst nach dem Abschluss bereit und
	// werden ueber GET binding geholt.
	switch resp.StatusCode {
	case http.StatusCreated:
		state.CreatedBindings = append(state.CreatedBindings, bindingRef{state.InstanceID, bindingID})
	case http.StatusAccepted:
		state.CreatedBindings = append(state.CreatedBindings, bindingRef{state.InstanceID, bindingID})
		st, err := s.client.WaitForOperation(state.InstanceID, bindingID, resp.Operation)
		if err != nil {
			s.recordFailure(results, testName+" (async)", "bind", err.Error(),
				"/v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation", "GET")
			return
		}
		if st != "succeeded" {
			s.recordFailure(results, testName+" (async)", "bind",
				fmt.Sprintf("last_operation ended in %q", st),
				"/v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation", "GET")
			return
		}
		s.recordSuccess(results, testName+" (async)", "bind")
		return
	default:
		s.recordFailure(results, testName, "bind",
			fmt.Sprintf("Expected status 201 or 202, got %d", resp.StatusCode),
			"/v2/service_instances/{instance_id}/service_bindings/{binding_id}", "PUT")
		return
	}

	if len(resp.Credentials) == 0 {
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

	bindingID := fmt.Sprintf("%s-bind-no-svc-%s", s.config.IDPrefix, runID())

	resp, err := s.client.BindInstance(state.InstanceID, bindingID, "", state.PlanID, "test-app")

	if err != nil || (resp != nil && isClientError(resp.StatusCode)) {
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

	bindingID := fmt.Sprintf("%s-bind-no-plan-%s", s.config.IDPrefix, runID())

	resp, err := s.client.BindInstance(state.InstanceID, bindingID, state.ServiceID, "", "test-app")

	if err != nil || (resp != nil && isClientError(resp.StatusCode)) {
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

	bindingID := fmt.Sprintf("%s-bind-ghost-%s", s.config.IDPrefix, runID())

	resp, err := s.client.BindInstance("nonexistent-instance", bindingID, state.ServiceID, state.PlanID, "test-app")

	if err != nil || (resp != nil && isClientError(resp.StatusCode)) {
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

	if err != nil || (resp != nil && isClientError(resp.StatusCode)) {
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

	if err != nil || (resp != nil && isClientError(resp.StatusCode)) {
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

	if err != nil || (resp != nil && isClientError(resp.StatusCode)) {
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
