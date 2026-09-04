package test

import (
	"net/http"

	"github.com/cyrano-janus/osb-checker/test/config"
	"github.com/cyrano-janus/osb-checker/test/models"
)

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

// TestSuite represents the complete test suite.
//
// Die Konfiguration wird nicht kopiert, sondern gehalten. Eine zweite Struktur
// mit denselben Feldern muss bei jeder Erweiterung von Hand nachgezogen
// werden, und genau das ging schief: accepts_async stand in der Datei, wurde
// beim Kopieren uebernommen und danach von niemandem mehr gelesen.
type TestSuite struct {
	config  *config.Config
	verbose bool
	client  *OSBClient
}

// OSBClient is the HTTP client for OSB API
type OSBClient struct {
	BaseURL    string
	Username   string
	Password   string
	APIVersion string

	// http traegt Timeout und TLS-Material. Ohne eigenen Client gaebe es
	// weder das eine noch das andere: http.Client{} wartet unbegrenzt und
	// prueft nur gegen die System-Roots.
	http *http.Client
	// acceptsAsync entscheidet, ob accepts_incomplete=true mitgeschickt wird.
	acceptsAsync bool
	// pollTimeoutSeconds begrenzt das Warten auf einen 202-Vorgang.
	pollTimeoutSeconds int
}

// Test state
type TestState struct {
	InstanceID string
	BindingID  string
	ServiceID  string
	PlanID     string
	Catalog    *models.Catalog

	// Angelegt wird getrennt vom Geprueften gefuehrt: aufgeraeumt werden muss
	// alles, was wirklich entstanden ist, auch wenn der Lauf danach abbricht.
	CreatedInstances []string
	CreatedBindings  []bindingRef
}

// bindingRef benennt ein Binding samt seiner Instanz - beides braucht das
// Unbind.
type bindingRef struct {
	InstanceID string
	BindingID  string
}
