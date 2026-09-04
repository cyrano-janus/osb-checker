package test

import (
	"strings"
	"testing"

	"github.com/cyrano-janus/osb-checker/test/config"
	"github.com/cyrano-janus/osb-checker/test/models"
)

// Welchen Service der Lauf anfasst, entscheidet ueber den Aussagewert des
// ganzen Berichts. Vorher stand hier Services[0] ohne Ausweg: die
// Katalogreihenfolge bestimmte, was geprueft wurde, und ein Service ohne
// Plaene liess den Checker mit einem Index-Panic abstuerzen.

func svc(id string, planIDs ...string) models.Service {
	s := models.Service{ID: id, Name: id}
	for _, p := range planIDs {
		s.Plans = append(s.Plans, models.ServicePlan{ID: p, Name: p})
	}
	return s
}

// pick baut eine Suite ohne HTTP und laesst sie waehlen.
func pick(t *testing.T, cfg *config.Config, svcs ...models.Service) (*TestResults, *TestState) {
	t.Helper()
	s := &TestSuite{config: cfg}
	results := &TestResults{}
	state := &TestState{Catalog: &models.Catalog{Services: svcs}}
	s.pickService(results, state)
	return results, state
}

func baseConfig() *config.Config {
	c := config.DefaultConfig()
	c.BrokerURL = "http://localhost:8080"
	return c
}

func TestPickService_NimmtDenErstenPassenden(t *testing.T) {
	_, state := pick(t, baseConfig(), svc("a", "a-free"), svc("b", "b-free"))

	if state.ServiceID != "a" || state.PlanID != "a-free" {
		t.Fatalf("got %s/%s, want a/a-free", state.ServiceID, state.PlanID)
	}
}

func TestPickService_UeberspringtWasInSkipServicesSteht(t *testing.T) {
	cfg := baseConfig()
	cfg.SkipServices = []string{"demo-1", "demo-2"}

	_, state := pick(t, cfg, svc("demo-1", "free"), svc("demo-2", "free"), svc("echt", "small"))

	if state.ServiceID != "echt" {
		t.Fatalf("got %q, want echt - skip_services muss die Demo-Angebote uebergehen", state.ServiceID)
	}
}

func TestPickService_VorgabeSchlaegtAutomatik(t *testing.T) {
	cfg := baseConfig()
	cfg.ServiceID, cfg.PlanID = "b", "b-large"

	results, state := pick(t, cfg, svc("a", "a-free"), svc("b", "b-small", "b-large"))

	if state.ServiceID != "b" || state.PlanID != "b-large" {
		t.Fatalf("got %s/%s, want b/b-large", state.ServiceID, state.PlanID)
	}
	if results.Failed != 0 {
		t.Fatalf("unerwarteter Fehlschlag: %+v", results.Failures)
	}
}

func TestPickService_UnbekannterServiceScheitertLaut(t *testing.T) {
	// Ein stiller Rueckfall waere hier das Schlimmste: der Bericht saehe
	// gruen aus fuer einen Service, den niemand pruefen wollte.
	cfg := baseConfig()
	cfg.ServiceID = "gibt-es-nicht"

	results, state := pick(t, cfg, svc("a", "a-free"))

	if state.ServiceID != "" {
		t.Fatalf("es darf nichts gewaehlt werden, gewaehlt wurde %q", state.ServiceID)
	}
	if results.Failed != 1 {
		t.Fatalf("Failed=%d, want 1", results.Failed)
	}
	if !strings.Contains(results.Failures[0].Error, "not in the catalog") {
		t.Fatalf("Meldung nennt die Ursache nicht: %q", results.Failures[0].Error)
	}
}

func TestPickService_PlanDerNichtDazugehoertScheitertLaut(t *testing.T) {
	cfg := baseConfig()
	cfg.ServiceID, cfg.PlanID = "a", "fremder-plan"

	results, _ := pick(t, cfg, svc("a", "a-free"))

	if results.Failed != 1 {
		t.Fatalf("Failed=%d, want 1", results.Failed)
	}
}

func TestPickService_ServiceOhnePlaeneWirdUebersprungen(t *testing.T) {
	// Frueher: Plans[0] ohne Laengenpruefung - Panic statt Bericht.
	_, state := pick(t, baseConfig(), svc("leer"), svc("voll", "p1"))

	if state.ServiceID != "voll" {
		t.Fatalf("got %q, want voll", state.ServiceID)
	}
}

func TestPickService_LeererKatalogMeldetDasAlsFehler(t *testing.T) {
	results, state := pick(t, baseConfig())

	if state.ServiceID != "" {
		t.Fatalf("es darf nichts gewaehlt werden, gewaehlt wurde %q", state.ServiceID)
	}
	if results.Failed != 1 {
		t.Fatalf("Failed=%d, want 1", results.Failed)
	}
}

func TestIsClientError_NurVierhundert(t *testing.T) {
	// Der Kommentar sagte 400-499, die Bedingung war >= 400: ein Broker, der
	// bei fehlender service_id mit 500 abstuerzt, bestand den Test.
	for _, tc := range []struct {
		status int
		want   bool
	}{{399, false}, {400, true}, {404, true}, {499, true}, {500, false}, {503, false}} {
		if got := isClientError(tc.status); got != tc.want {
			t.Errorf("isClientError(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestRunID_IstJeLaufVerschieden(t *testing.T) {
	// Feste IDs liessen zwei gleichzeitige Laeufe auf derselben Instanz
	// arbeiten.
	if runID() == runID() {
		t.Fatal("runID liefert zweimal denselben Wert")
	}
}
