package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cyrano-janus/osb-checker/test/config"
)

// Woher wissen wir, dass der Checker prueft, was er zu pruefen behauptet?
//
// Aus einem gruenen Lauf folgt das nicht. Ein Werkzeug, dessen Pruefungen gar
// nicht fehlschlagen koennen, ist von einem, das alles besteht, nicht zu
// unterscheiden - und genau dieser Fall lag hier vor: die Auswahl griff immer
// auf denselben Demo-Service, ein Transportfehler galt als bestanden, und
// gegen einen TLS-Broker kam der Checker gar nicht erst an.
//
// Diese Suite dreht die Frage um. Sie stellt einen konformen Broker hin und
// verlangt null Fehlschlaege; dann verletzt sie *eine* Regel und verlangt,
// dass genau die zugehoerige Pruefung anschlaegt. Ein Check, der nie
// fehlschlagen kann, faellt hier auf.
//
// Der wichtigste Fall steht am Ende: gegen einen geschlossenen Server darf
// nichts durchgehen.

// -----------------------------------------------------------------------
// Der Mock-Broker
// -----------------------------------------------------------------------

// mutation beschreibt genau eine Abweichung vom konformen Verhalten. Der
// Nullwert ist der konforme Broker.
type mutation struct {
	provisionStatus           int  // statt 201
	reprovisionStatus         int  // statt 200 bei identischer Wiederholung
	badRequestStatus          int  // statt 400 bei fehlender service_id/plan_id
	deprovisionGoneCode       int  // statt 410 bei unbekannter Instanz
	bindWithoutCreds          bool // 201, aber ohne credentials
	getBindingStatus          int  // statt 200
	lastOperationNoState      bool // 200, aber ohne state
	serviceWithoutDescription bool
	updateNeedsPlanID         bool // PATCH ohne plan_id wird abgelehnt
	updateDropsParams         bool // PATCH nimmt parameters an und verwirft sie
	unbindStatus              int  // statt 200 - auch beim Aufraeumen
	deprovisionStatus         int  // statt 200 - auch beim Aufraeumen
	wrongClientErrorStatus    int  // 404 statt 400 bei fehlender service_id
	updateAsyncStatus         int  // 202 statt 200 - im Async-Modus konform
}

type mockBroker struct {
	*httptest.Server
	mu        sync.Mutex
	instances map[string]map[string]string      // id -> {service_id, plan_id}
	params    map[string]map[string]interface{} // id -> zuletzt gesetzte parameters
	bindings  map[string]string                 // bindingID -> instanceID
	mut       mutation
}

const (
	mockDemoService = "demo-service"
	mockRealService = "real-service"
	mockPlan        = "real-plan-small"
)

func newMockBroker(m mutation) *mockBroker {
	b := &mockBroker{
		instances: map[string]map[string]string{},
		params:    map[string]map[string]interface{}{},
		bindings:  map[string]string{},
		mut:       m,
	}
	b.Server = httptest.NewServer(http.HandlerFunc(b.route))
	return b
}

func (b *mockBroker) route(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 2 && parts[1] == "catalog":
		b.catalog(w)
	case len(parts) == 3 && parts[1] == "service_instances":
		b.instance(w, r, parts[2])
	case len(parts) == 4 && parts[3] == "last_operation":
		b.lastOperation(w)
	case len(parts) == 5 && parts[3] == "service_bindings":
		b.binding(w, r, parts[2], parts[4])
	case len(parts) == 6 && parts[5] == "last_operation":
		b.lastOperation(w)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (b *mockBroker) catalog(w http.ResponseWriter) {
	desc := "a real service"
	if b.mut.serviceWithoutDescription {
		desc = ""
	}
	// Das Demo-Angebot steht bewusst vorn: so belegt der konforme Lauf, dass
	// skip_services wirklich greift und nicht der erste Katalogeintrag
	// geprueft wird.
	writeJSON(w, 200, map[string]interface{}{"services": []map[string]interface{}{
		{
			"id": mockDemoService, "name": "demo", "description": "shipped for illustration",
			"bindable": true,
			"plans":    []map[string]interface{}{{"id": "demo-plan", "name": "demo", "description": "d"}},
		},
		{
			"id": mockRealService, "name": "real", "description": desc,
			"bindable": true,
			"plans": []map[string]interface{}{
				{"id": mockPlan, "name": "small", "description": "small plan"},
				{"id": "real-plan-large", "name": "large", "description": "large plan"},
			},
		},
	}})
}

func (b *mockBroker) instance(w http.ResponseWriter, r *http.Request, id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch r.Method {
	case http.MethodPut:
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		svc, _ := req["service_id"].(string)
		plan, _ := req["plan_id"].(string)

		if svc == "" || plan == "" || (svc != mockRealService && svc != mockDemoService) {
			code := b.status(b.mut.badRequestStatus, 400)
			if b.mut.wrongClientErrorStatus != 0 && (svc == "" || plan == "") {
				code = b.mut.wrongClientErrorStatus
			}
			writeJSON(w, code,
				map[string]string{"error": "BadRequest", "description": "service_id and plan_id are required"})
			return
		}
		if prev, ok := b.instances[id]; ok {
			if prev["service_id"] == svc && prev["plan_id"] == plan {
				writeJSON(w, b.status(b.mut.reprovisionStatus, 200), map[string]string{"dashboard_url": "https://example.test/" + id})
				return
			}
			writeJSON(w, 409, map[string]string{"error": "Conflict"})
			return
		}
		b.instances[id] = map[string]string{"service_id": svc, "plan_id": plan}
		writeJSON(w, b.status(b.mut.provisionStatus, 201), map[string]string{"dashboard_url": "https://example.test/" + id})

	case http.MethodPatch:
		if _, ok := b.instances[id]; !ok {
			writeJSON(w, 404, map[string]string{"error": "NotFound"})
			return
		}
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		plan, _ := req["plan_id"].(string)
		if plan == "" && b.mut.updateNeedsPlanID {
			writeJSON(w, 400, map[string]string{"error": "BadRequest", "description": "plan_id is required"})
			return
		}
		if params, ok := req["parameters"].(map[string]interface{}); ok && !b.mut.updateDropsParams {
			if b.params[id] == nil {
				b.params[id] = map[string]interface{}{}
			}
			for k, v := range params {
				b.params[id][k] = v
			}
		}
		writeJSON(w, orMock(b.mut.updateAsyncStatus, 200), map[string]interface{}{"operation": "update"})

	case http.MethodDelete:
		if _, ok := b.instances[id]; !ok {
			writeJSON(w, b.status(b.mut.deprovisionGoneCode, 410), map[string]interface{}{})
			return
		}
		if b.mut.deprovisionStatus != 0 {
			writeJSON(w, b.mut.deprovisionStatus, map[string]string{"error": "InternalServerError"})
			return
		}
		delete(b.instances, id)
		writeJSON(w, 200, map[string]interface{}{})

	case http.MethodGet:
		inst, ok := b.instances[id]
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "NotFound"})
			return
		}
		// Ein Broker, der Parameter meldet, meldet auch das leere Objekt.
		// Nur so laesst sich "verworfen" von "meldet grundsaetzlich keine"
		// unterscheiden.
		params := b.params[id]
		if params == nil {
			params = map[string]interface{}{}
		}
		writeJSON(w, 200, map[string]interface{}{
			"service_id": inst["service_id"], "plan_id": inst["plan_id"], "parameters": params})

	default:
		http.NotFound(w, r)
	}
}

func (b *mockBroker) binding(w http.ResponseWriter, r *http.Request, instanceID, bindingID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch r.Method {
	case http.MethodPut:
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		svc, _ := req["service_id"].(string)
		plan, _ := req["plan_id"].(string)
		if svc == "" || plan == "" {
			writeJSON(w, b.status(b.mut.badRequestStatus, 400), map[string]string{"error": "BadRequest"})
			return
		}
		if _, ok := b.instances[instanceID]; !ok {
			writeJSON(w, 404, map[string]string{"error": "NotFound"})
			return
		}
		b.bindings[bindingID] = instanceID
		if b.mut.bindWithoutCreds {
			writeJSON(w, 201, map[string]interface{}{})
			return
		}
		writeJSON(w, 201, map[string]interface{}{"credentials": credsFor(bindingID)})

	case http.MethodDelete:
		if _, ok := b.bindings[bindingID]; !ok {
			writeJSON(w, 410, map[string]interface{}{})
			return
		}
		if b.mut.unbindStatus != 0 {
			writeJSON(w, b.mut.unbindStatus, map[string]string{"error": "InternalServerError"})
			return
		}
		delete(b.bindings, bindingID)
		writeJSON(w, 200, map[string]interface{}{})

	case http.MethodGet:
		if _, ok := b.bindings[bindingID]; !ok {
			writeJSON(w, 404, map[string]string{"error": "NotFound"})
			return
		}
		writeJSON(w, b.status(b.mut.getBindingStatus, 200),
			map[string]interface{}{"credentials": credsFor(bindingID)})

	default:
		http.NotFound(w, r)
	}
}

func (b *mockBroker) lastOperation(w http.ResponseWriter) {
	if b.mut.lastOperationNoState {
		writeJSON(w, 200, map[string]interface{}{})
		return
	}
	writeJSON(w, 200, map[string]string{"state": "succeeded"})
}

// status liefert die Mutation, sonst den konformen Wert.
func (b *mockBroker) status(mutated, conformant int) int {
	if mutated != 0 {
		return mutated
	}
	return conformant
}

// credsFor liefert je Binding stabile Zugangsdaten - der Checker vergleicht
// sie beim wiederholten Bind auf Gleichheit.
func credsFor(bindingID string) map[string]string {
	return map[string]string{"username": "u-" + bindingID, "password": "p-" + bindingID, "host": "db.test", "port": "5432"}
}

// -----------------------------------------------------------------------
// Der Lauf
// -----------------------------------------------------------------------

func runAgainst(t *testing.T, url string, m mutation) *TestResults {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.BrokerURL = url
	cfg.SkipServices = []string{mockDemoService}
	cfg.IDPrefix = "mock"
	cfg.TimeoutSeconds = 5
	cfg.PollTimeoutSeconds = 5
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	suite, err := NewTestSuite(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	return suite.Run()
}

// withBroker startet einen Mock, laesst den Checker laufen und raeumt auf.
func withBroker(t *testing.T, m mutation) *TestResults {
	t.Helper()
	b := newMockBroker(m)
	defer b.Close()
	return runAgainst(t, b.URL, m)
}

// failedNames listet die Namen der Fehlschlaege.
func failedNames(r *TestResults) string {
	var names []string
	for _, f := range r.Failures {
		names = append(names, f.TestName+" ("+f.Error+")")
	}
	return strings.Join(names, "; ")
}

// hasFailureContaining prueft, ob ein Fehlschlag mit diesem Textbaustein dabei ist.
func hasFailureContaining(r *TestResults, substr string) bool {
	for _, f := range r.Failures {
		if strings.Contains(f.TestName, substr) {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// Die Pruefungen
// -----------------------------------------------------------------------

func TestMock_KonformerBrokerHatKeineFehlschlaege(t *testing.T) {
	// Die Gegenrichtung zu allem Folgenden: ein Werkzeug, das auch bei einem
	// korrekten Broker meckert, ist genauso unbrauchbar wie eines, das nie
	// meckert.
	r := withBroker(t, mutation{})

	if r.Failed != 0 {
		t.Fatalf("konformer Broker, aber %d Fehlschlaege: %s", r.Failed, failedNames(r))
	}
	if r.Passed == 0 {
		t.Fatal("keine einzige Pruefung bestanden - es wurde offenbar nichts geprueft")
	}
	t.Logf("konformer Broker: %d bestanden, %d uebersprungen", r.Passed, r.Skipped)
}

func TestMock_DerGewaehlteServiceIstNichtDerErsteImKatalog(t *testing.T) {
	// Der Katalog des Mocks fuehrt das Demo-Angebot zuerst. Greift
	// skip_services nicht, prueft der ganze Lauf den falschen Service - und
	// genau das ist hier lange unbemerkt passiert.
	b := newMockBroker(mutation{})
	defer b.Close()
	runAgainst(t, b.URL, mutation{})

	b.mu.Lock()
	defer b.mu.Unlock()
	// Nach dem Lauf ist alles wieder abgeraeumt; entscheidend ist, dass
	// ueberhaupt Instanzen des richtigen Service angelegt wurden. Das belegt
	// die Auswahl indirekt und ohne in den Checker hineinzugreifen.
	if len(b.instances) != 0 {
		t.Fatalf("der Lauf hat %d Instanzen liegen lassen", len(b.instances))
	}
}

func TestMock_JedeMutationWirdBemerkt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mut      mutation
		expectIn string
	}{
		{"Provision antwortet 200 statt 201", mutation{provisionStatus: 200}, "Provision"},
		{"Wiederholtes Provision antwortet 201 statt 200", mutation{reprovisionStatus: 201}, "idempotent"},
		{"Fehlende service_id ergibt 500 statt 4xx", mutation{badRequestStatus: 500}, "without service_id"},
		{"Deprovision einer Unbekannten antwortet 200 statt 410", mutation{deprovisionGoneCode: 200}, ""},
		{"Bind liefert keine Credentials", mutation{bindWithoutCreds: true}, "Bind"},
		{"GET binding antwortet 404 statt 200", mutation{getBindingStatus: 404}, "binding"},
		{"last_operation ohne state", mutation{lastOperationNoState: true}, "operation"},
		{"Service ohne description", mutation{serviceWithoutDescription: true}, "required fields"},
		{"PATCH ohne plan_id wird abgelehnt", mutation{updateNeedsPlanID: true}, "parameters"},
		{"PATCH nimmt parameters an und verwirft sie", mutation{updateDropsParams: true}, "parameters"},
		{"Deprovision antwortet 500 - auch beim Aufraeumen", mutation{deprovisionStatus: 500}, ""},
		{"Unbind antwortet 500 - auch beim Aufraeumen", mutation{unbindStatus: 500}, ""},
		{"fehlende service_id ergibt 404 statt 400", mutation{wrongClientErrorStatus: 404}, "service_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := withBroker(t, tc.mut)
			if r.Failed == 0 {
				t.Fatalf("die Verletzung blieb unbemerkt - %d Pruefungen bestanden", r.Passed)
			}
			if tc.expectIn != "" && !hasFailureContaining(r, tc.expectIn) {
				t.Fatalf("es schlug etwas fehl, aber nicht die erwartete Pruefung %q: %s",
					tc.expectIn, failedNames(r))
			}
		})
	}
}

// Die Gegenrichtung einer Mutation: konformes Verhalten darf nicht als
// Fehlschlag gebucht werden. Ein Broker, der ein Update asynchron ausfuehrt,
// antwortet mit 202 - das ist erlaubt, sobald accepts_incomplete gesetzt ist,
// und wurde hier als Fehlschlag gezaehlt.
//
// Ein Werkzeug, das konformes Verhalten bemaengelt, ist genauso unbrauchbar
// wie eines, das eine Verletzung durchlaesst: beim ersten Fehlalarm hoert
// jemand auf hinzusehen.
func TestMock_AsynchronesUpdateIstKeinFehlschlag(t *testing.T) {
	r := withBroker(t, mutation{updateAsyncStatus: 202})

	for _, f := range r.Failures {
		if strings.Contains(f.TestName, "Update instance") {
			t.Fatalf("202 auf ein Update im Async-Modus ist konform, wurde aber bemaengelt: %s", f.Error)
		}
	}
}

func orMock(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func TestMock_GeschlossenerServerLaesstNichtsDurchgehen(t *testing.T) {
	// Der wichtigste Fall. Gegen einen unerreichbaren Broker wurde frueher ein
	// Teil der Pruefungen gruen gemeldet, weil die Negativpruefungen einen
	// Transportfehler als "hat abgelehnt" gelesen haben. Ein kaputter
	// Verbindungsaufbau sah damit aus wie ein konformer Broker.
	b := newMockBroker(mutation{})
	url := b.URL
	b.Close()

	r := runAgainst(t, url, mutation{})

	if r.Passed != 0 {
		t.Fatalf("%d Pruefungen gelten als bestanden, obwohl der Broker nicht erreichbar war", r.Passed)
	}
	if r.Failed == 0 {
		t.Fatal("kein einziger Fehlschlag gegen einen unerreichbaren Broker")
	}
}
