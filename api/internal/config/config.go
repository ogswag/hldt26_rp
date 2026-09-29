package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL       string
	APIPort           string
	PublicOrigin      string
	CookieSecure      bool
	SessionIdle       time.Duration
	SessionMax        time.Duration
	CatalogCSV        string
	CatalogSpecs      string
	MaxConns          int32
	BcryptCost        int
	DemoUserEmail     string
	DemoUserPassword  string
	DemoAdminEmail    string
	DemoAdminPassword string
	SimWorkers        int
	SimJobTimeout     time.Duration
	SimActivePerUser  int
	SimLogRetention   time.Duration
	AppEnv            string
	MailMode          string
	MailFrom          string
	SMTPHost          string
	SMTPPort          int
	SMTPUser          string
	SMTPPassword      string
	RegistrationMode  string
	TrashRetention    time.Duration
	EngineDir         string
	AuditRetention    time.Duration
}

const (
	MailCapture        = "capture"
	MailSMTP           = "smtp"
	RegistrationOpen   = "open"
	RegistrationInvite = "invite"
)

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		APIPort:           getenv("API_PORT", "8080"),
		PublicOrigin:      getenv("PUBLIC_ORIGIN", "http://localhost"),
		CookieSecure:      true,
		CatalogCSV:        getenv("CATALOG_CSV", "/data/catalog.csv"),
		CatalogSpecs:      getenv("CATALOG_SPECS", "/data/seeds/robot_specs.json"),
		MaxConns:          10,
		BcryptCost:        10,
		DemoUserEmail:     getenv("DEMO_USER_EMAIL", "user@demo.local"),
		DemoUserPassword:  os.Getenv("DEMO_USER_PASSWORD"),
		DemoAdminEmail:    getenv("DEMO_ADMIN_EMAIL", "admin@demo.local"),
		DemoAdminPassword: os.Getenv("DEMO_ADMIN_PASSWORD"),
		SimWorkers:        2,
		SimJobTimeout:     10 * time.Minute,
		SimActivePerUser:  3,
		SimLogRetention:   90 * 24 * time.Hour,
		AppEnv:            getenv("APP_ENV", "local"),
		MailMode:          getenv("MAIL_MODE", MailCapture),
		MailFrom:          getenv("MAIL_FROM", "no-reply@localhost"),
		SMTPHost:          os.Getenv("SMTP_HOST"),
		SMTPPort:          587,
		SMTPUser:          os.Getenv("SMTP_USER"),
		SMTPPassword:      os.Getenv("SMTP_PASSWORD"),
		RegistrationMode:  getenv("REGISTRATION_MODE", RegistrationOpen),
		EngineDir:         getenv("ENGINE_DIR", "/engine"),
		AuditRetention:    365 * 24 * time.Hour,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	switch os.Getenv("COOKIE_SECURE") {
	case "", "true":
	case "false":
		cfg.CookieSecure = false
	default:
		return Config{}, fmt.Errorf("config: COOKIE_SECURE must be true or false")
	}
	if v := os.Getenv("PGPOOL_MAX_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("config: PGPOOL_MAX_CONNS must be a positive integer")
		}
		cfg.MaxConns = int32(n)
	}
	if v := os.Getenv("BCRYPT_COST"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 10 || n > 12 {
			return Config{}, fmt.Errorf("config: BCRYPT_COST must be 10, 11 or 12")
		}
		cfg.BcryptCost = n
	}
	n, err := intEnv("SESSION_IDLE_HOURS", 168, 1, 8760)
	if err != nil {
		return Config{}, err
	}
	cfg.SessionIdle = time.Duration(n) * time.Hour
	if n, err = intEnv("SESSION_MAX_DAYS", 30, 1, 365); err != nil {
		return Config{}, err
	}
	cfg.SessionMax = time.Duration(n) * 24 * time.Hour
	if n, err = intEnv("SIM_WORKERS", 2, 1, 8); err != nil {
		return Config{}, err
	}
	cfg.SimWorkers = n
	if n, err = intEnv("SIM_JOB_TIMEOUT_MIN", 10, 1, 60); err != nil {
		return Config{}, err
	}
	cfg.SimJobTimeout = time.Duration(n) * time.Minute
	if n, err = intEnv("SIM_MAX_ACTIVE_PER_USER", 3, 1, 20); err != nil {
		return Config{}, err
	}
	cfg.SimActivePerUser = n
	if n, err = intEnv("SIM_LOG_RETENTION_DAYS", 90, 1, 3650); err != nil {
		return Config{}, err
	}
	cfg.SimLogRetention = time.Duration(n) * 24 * time.Hour
	if n, err = intEnv("TRASH_RETENTION_DAYS", 30, 1, 3650); err != nil {
		return Config{}, err
	}
	cfg.TrashRetention = time.Duration(n) * 24 * time.Hour
	if n, err = intEnv("SMTP_PORT", 587, 1, 65535); err != nil {
		return Config{}, err
	}
	cfg.SMTPPort = n
	switch cfg.MailMode {
	case MailCapture:
	case MailSMTP:
		if cfg.SMTPHost == "" {
			return Config{}, fmt.Errorf("config: SMTP_HOST is required when MAIL_MODE is smtp")
		}
	default:
		return Config{}, fmt.Errorf("config: MAIL_MODE must be capture or smtp")
	}
	switch cfg.RegistrationMode {
	case RegistrationOpen, RegistrationInvite:
	default:
		return Config{}, fmt.Errorf("config: REGISTRATION_MODE must be open or invite")
	}
	return cfg, nil
}

func intEnv(key string, def, min, max int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("config: %s must be an integer from %d to %d", key, min, max)
	}
	return n, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
