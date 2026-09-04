package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Die Konfiguration hat gelogen: weggelassene test_*-Schluessel fielen auf
// false, weil Go Bools so nullt - die Datei bedeutete also das Gegenteil
// dessen, was DefaultConfig versprach. Und accepts_async stand zwar in der
// Datei, wurde aber von niemandem gelesen.

// fromMap macht aus einer Tabelle eine Lookup-Funktion, wie ApplyEnv sie
// erwartet. Tests fassen damit nie die Prozessumgebung an.
func fromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_WeggelasseneSchluesselBehaltenDenStandard(t *testing.T) {
	cfg, err := LoadConfig(write(t, "broker_url: http://localhost:8080\n"))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		got  bool
	}{
		{"test_catalog", cfg.TestCatalog},
		{"test_provision", cfg.TestProvision},
		{"test_bind", cfg.TestBind},
		{"test_update", cfg.TestUpdate},
		{"test_fetch", cfg.TestFetch},
		{"accepts_async", cfg.AcceptsAsync},
	} {
		if !tc.got {
			t.Errorf("%s ist false, obwohl der Schluessel fehlt - der Standard ist true", tc.name)
		}
	}
	if cfg.APIVersion != "2.17" {
		t.Errorf("api_version = %q, want 2.17", cfg.APIVersion)
	}
	if cfg.TimeoutSeconds != 30 || cfg.PollTimeoutSeconds != 300 {
		t.Errorf("Zeitschranken nicht gesetzt: %d/%d", cfg.TimeoutSeconds, cfg.PollTimeoutSeconds)
	}
}

func TestLoad_GesetzteSchluesselGewinnen(t *testing.T) {
	cfg, err := LoadConfig(write(t, `
broker_url: http://localhost:8080
accepts_async: false
test_bind: false
service_id: svc-1
plan_id: plan-1
skip_services: [demo-1, demo-2]
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AcceptsAsync {
		t.Error("accepts_async: false wurde nicht uebernommen")
	}
	if cfg.TestBind {
		t.Error("test_bind: false wurde nicht uebernommen")
	}
	if cfg.ServiceID != "svc-1" || cfg.PlanID != "plan-1" {
		t.Errorf("Auswahl nicht uebernommen: %s/%s", cfg.ServiceID, cfg.PlanID)
	}
	if len(cfg.SkipServices) != 2 {
		t.Errorf("skip_services = %v", cfg.SkipServices)
	}
}

func TestLoad_SchraegstrichAmEndeFaelltWeg(t *testing.T) {
	// Sonst entsteht //v2/catalog, und das hat mit Konformitaet nichts zu tun.
	cfg, err := LoadConfig(write(t, "broker_url: http://localhost:8080/\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BrokerURL != "http://localhost:8080" {
		t.Errorf("broker_url = %q", cfg.BrokerURL)
	}
}

func TestApplyEnv_UeberschreibtDieDatei(t *testing.T) {
	// In einer CI gehoeren Zugangsdaten nicht in eine eingecheckte Datei.
	cfg := DefaultConfig()
	cfg.BrokerURL = "http://aus-der-datei"
	cfg.Username = "datei-user"

	cfg.ApplyEnv(fromMap(map[string]string{
		"OSB_BROKER_URL": "https://aus-der-umgebung",
		"OSB_PASSWORD":   "geheim",
	}))

	if cfg.BrokerURL != "https://aus-der-umgebung" {
		t.Errorf("broker_url = %q", cfg.BrokerURL)
	}
	if cfg.Password != "geheim" {
		t.Errorf("password nicht uebernommen")
	}
	if cfg.Username != "datei-user" {
		t.Errorf("eine nicht gesetzte Variable darf die Datei nicht leeren, username = %q", cfg.Username)
	}
}

func TestValidate_MeldetUnbrauchbareKombinationen(t *testing.T) {
	for _, tc := range []struct {
		name string
		fix  func(*Config)
	}{
		{"ohne broker_url", func(c *Config) { c.BrokerURL = "" }},
		{"plan_id ohne service_id", func(c *Config) { c.PlanID = "p" }},
		{"timeout 0", func(c *Config) { c.TimeoutSeconds = 0 }},
		{"nur client_cert", func(c *Config) { c.ClientCert = "a.pem" }},
	} {
		cfg := DefaultConfig()
		cfg.BrokerURL = "http://localhost:8080"
		tc.fix(cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: Validate akzeptiert die Konfiguration", tc.name)
		}
	}
}
