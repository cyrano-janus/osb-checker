package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config represents the test configuration.
//
// Absent keys keep the values from DefaultConfig: LoadConfig unmarshals into a
// pre-filled struct. Ohne das faellt ein weggelassenes test_catalog auf false,
// und die Kategorie wird stillschweigend uebersprungen - die Datei bedeutete
// dann etwas anderes, als sie sagt.
type Config struct {
	BrokerURL  string `yaml:"broker_url"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	APIVersion string `yaml:"api_version"`

	// AcceptsAsync schickt accepts_incomplete=true als Query-Parameter. Der
	// Broker darf dann mit 202 antworten; der Checker pollt anschliessend
	// last_operation, bis der Vorgang abgeschlossen ist.
	AcceptsAsync bool `yaml:"accepts_async"`
	// PollTimeoutSeconds begrenzt dieses Warten.
	PollTimeoutSeconds int `yaml:"poll_timeout_seconds"`

	// ServiceID und PlanID waehlen, was geprueft wird. Ohne Vorgabe nimmt der
	// Checker den ersten Service des Katalogs, der nicht in SkipServices steht
	// und mindestens einen Plan hat.
	//
	// Vorgeben ist der Regelfall: welcher Service vorn steht, entscheidet
	// sonst die Reihenfolge im Katalog, und die ist keine Zusage.
	ServiceID string `yaml:"service_id"`
	PlanID    string `yaml:"plan_id"`
	// SkipServices nennt Angebote, die bei der automatischen Wahl uebergangen
	// werden - etwa Demo-Services, die ein Broker mitliefert.
	SkipServices []string `yaml:"skip_services"`

	// IDPrefix geht in jede erzeugte Instanz- und Binding-ID ein. Zwei
	// gleichzeitige Laeufe gegen denselben Broker duerfen sich nicht auf
	// dieselbe Instanz setzen.
	IDPrefix string `yaml:"id_prefix"`

	// TimeoutSeconds begrenzt jeden einzelnen HTTP-Request. Ohne Timeout
	// haengt ein Lauf an einem stummen Broker unbegrenzt.
	TimeoutSeconds int `yaml:"timeout_seconds"`

	// Resolve bildet einen Hostnamen auf eine Adresse ab, in der Schreibweise
	// von curls --resolve: "host:port:adresse".
	//
	// Ein Broker hinter einem Port-Forward ist unter localhost erreichbar,
	// sein Zertifikat lautet aber auf den Service-Namen im Cluster. Ohne diese
	// Abbildung bliebe nur, die Zertifikatspruefung abzuschalten - und ein
	// Aufruf, der das Zertifikat nicht prueft, prueft gar nichts.
	Resolve string `yaml:"resolve"`

	// TLS. CACert prueft das Broker-Zertifikat, ClientCert/ClientKey weisen
	// den Checker per mTLS aus. Insecure schaltet die Pruefung ab und ist nur
	// zum Eingrenzen gedacht.
	CACert     string `yaml:"ca_cert"`
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
	Insecure   bool   `yaml:"insecure"`

	TestCatalog   bool `yaml:"test_catalog"`
	TestProvision bool `yaml:"test_provision"`
	TestBind      bool `yaml:"test_bind"`
	TestUpdate    bool `yaml:"test_update"`
	TestFetch     bool `yaml:"test_fetch"`
}

// DefaultConfig returns the configuration a caller gets without a file.
func DefaultConfig() *Config {
	return &Config{
		APIVersion:         "2.17",
		AcceptsAsync:       true,
		PollTimeoutSeconds: 300,
		IDPrefix:           "osb-checker",
		TimeoutSeconds:     30,
		TestCatalog:        true,
		TestProvision:      true,
		TestBind:           true,
		TestUpdate:         true,
		TestFetch:          true,
	}
}

// LoadConfig loads configuration from a YAML file and applies the environment
// overrides.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.ApplyEnv(os.Getenv)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ApplyEnv laesst Umgebungsvariablen die Datei ueberschreiben. Zugangsdaten
// gehoeren in einer CI nicht in eine eingecheckte Datei, und eine
// Umgebungsvariable ist der uebliche Weg dorthin.
func (c *Config) ApplyEnv(get func(string) string) {
	for _, o := range []struct {
		key string
		dst *string
	}{
		{"OSB_BROKER_URL", &c.BrokerURL},
		{"OSB_USERNAME", &c.Username},
		{"OSB_PASSWORD", &c.Password},
		{"OSB_SERVICE_ID", &c.ServiceID},
		{"OSB_PLAN_ID", &c.PlanID},
		{"OSB_ID_PREFIX", &c.IDPrefix},
	} {
		if v := get(o.key); v != "" {
			*o.dst = v
		}
	}
}

// Validate rejects a configuration that cannot produce a meaningful run.
func (c *Config) Validate() error {
	if c.BrokerURL == "" {
		return fmt.Errorf("broker_url is required")
	}
	// Ein Schraegstrich am Ende ergaebe //v2/catalog. Manche Broker
	// beantworten das, andere nicht - der Unterschied hat mit Konformitaet
	// nichts zu tun und soll niemandem Zeit kosten.
	c.BrokerURL = strings.TrimSuffix(c.BrokerURL, "/")

	if c.PlanID != "" && c.ServiceID == "" {
		return fmt.Errorf("plan_id without service_id: a plan is only meaningful within its service")
	}
	if c.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeout_seconds must be positive")
	}
	if c.PollTimeoutSeconds <= 0 {
		return fmt.Errorf("poll_timeout_seconds must be positive")
	}
	if (c.ClientCert == "") != (c.ClientKey == "") {
		return fmt.Errorf("client_cert and client_key must be set together")
	}
	return nil
}
