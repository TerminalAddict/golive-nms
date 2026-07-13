package app

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golive-nms/golive-nms/internal/api"
	configengine "github.com/golive-nms/golive-nms/internal/configbackup"
	deviceevents "github.com/golive-nms/golive-nms/internal/events"
	"github.com/golive-nms/golive-nms/internal/monitor"
	"github.com/golive-nms/golive-nms/internal/notifier"
	"github.com/golive-nms/golive-nms/internal/pki"
	"github.com/golive-nms/golive-nms/internal/sso"
	"github.com/golive-nms/golive-nms/internal/store"
)

//go:embed migrations/*.sql
var migrations embed.FS

//go:embed all:web/dist
var webAssets embed.FS

type Config struct {
	Listen, CollectorListen, SyslogUDPListen, SyslogTCPListen, TrapListen, Domain, DatabaseURL, MetricsURL, LogsURL, AdminEmail, AdminPassword, AgentToken, MonitUsername, MonitPassword, OIDCIssuer, OIDCClientID, OIDCClientSecret, OIDCRedirectURL string
	RetentionDays                                                                                                                                                                                                                                     int
}

func ConfigFromEnv() Config {
	value := func(k, fallback string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return fallback
	}
	retentionDays, _ := strconv.Atoi(value("GOLIVE_RETENTION_DAYS", "395"))
	return Config{
		Listen: value("GOLIVE_LISTEN", ":8080"), DatabaseURL: os.Getenv("GOLIVE_DATABASE_URL"),
		CollectorListen: value("GOLIVE_COLLECTOR_LISTEN", ":8443"), Domain: value("GOLIVE_DOMAIN", "localhost"),
		SyslogUDPListen: value("GOLIVE_SYSLOG_UDP_LISTEN", ":5514"), SyslogTCPListen: value("GOLIVE_SYSLOG_TCP_LISTEN", ":5514"), TrapListen: value("GOLIVE_TRAP_LISTEN", ":1162"),
		MetricsURL: value("GOLIVE_METRICS_URL", "http://localhost:8428"), LogsURL: value("GOLIVE_LOGS_URL", "http://localhost:9428"),
		AdminEmail: value("GOLIVE_ADMIN_EMAIL", "admin@example.com"), AdminPassword: os.Getenv("GOLIVE_ADMIN_PASSWORD"),
		AgentToken:    os.Getenv("GOLIVE_AGENT_TOKEN"),
		MonitUsername: value("GOLIVE_MONIT_USERNAME", "monit"), MonitPassword: os.Getenv("GOLIVE_MONIT_PASSWORD"),
		OIDCIssuer: os.Getenv("GOLIVE_OIDC_ISSUER"), OIDCClientID: os.Getenv("GOLIVE_OIDC_CLIENT_ID"), OIDCClientSecret: os.Getenv("GOLIVE_OIDC_CLIENT_SECRET"), OIDCRedirectURL: os.Getenv("GOLIVE_OIDC_REDIRECT_URL"),
		RetentionDays: retentionDays,
	}
}

type App struct {
	Config       Config
	Store        *store.Store
	Monitor      *monitor.Engine
	API          *api.API
	Authority    *pki.Authority
	Events       *deviceevents.Receiver
	ConfigBackup *configengine.Engine
	Notifier     *notifier.Notifier
	logger       *slog.Logger
}

func New(ctx context.Context, cfg Config, logger *slog.Logger) (*App, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("GOLIVE_DATABASE_URL is required")
	}
	db, err := store.Open(ctx, cfg.DatabaseURL, migrations, os.Getenv("GOLIVE_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	if err = db.EnsureAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		db.Close()
		return nil, fmt.Errorf("bootstrap administrator: %w", err)
	}
	authority, err := pki.LoadOrCreate(ctx, db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize PKI: %w", err)
	}
	oidc, err := sso.New(ctx, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize OIDC: %w", err)
	}
	bus := api.NewEventBus()
	notify := notifier.New(db)
	notifyFn := func(title, deviceID, device, state string) {
		go notify.Publish(context.Background(), notifier.Event{Title: title, DeviceID: deviceID, DeviceName: device, State: state})
	}
	engine := monitor.New(db, bus.Publish, logger, func(_ context.Context, title, deviceID, device, state string) {
		notifyFn(title, deviceID, device, state)
	}, cfg.MetricsURL)
	receiver := deviceevents.New(db, logger, cfg.SyslogUDPListen, cfg.SyslogTCPListen, cfg.TrapListen)
	backups := configengine.New(db, logger, notifyFn)
	return &App{Config: cfg, Store: db, Monitor: engine, API: api.New(db, bus, logger, cfg.AgentToken, cfg.MonitUsername, cfg.MonitPassword, cfg.MetricsURL, authority, notifyFn, oidc), Authority: authority, Events: receiver, ConfigBackup: backups, Notifier: notify, logger: logger}, nil
}

func (a *App) Close() { a.Store.Close() }
func (a *App) CollectorTLSConfig() (*tls.Config, error) {
	return a.Authority.ServerTLS(a.Config.Domain)
}
func (a *App) CollectorHandler() http.Handler {
	full := a.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collector" && r.URL.Path != "/api/v1/agent/report" && !strings.HasPrefix(r.URL.Path, "/api/v1/agent/actions") && !strings.HasPrefix(r.URL.Path, "/api/v1/collector/") {
			http.NotFound(w, r)
			return
		}
		full.ServeHTTP(w, r)
	})
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	a.API.Routes(mux)
	dist, err := fs.Sub(webAssets, "web/dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(dist, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
	return securityHeaders(requestLog(a.authenticate(mux), a.logger))
}

func (a *App) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || r.URL.Path == "/api/v1/agent/report" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/actions") || r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/config" || strings.HasPrefix(r.URL.Path, "/api/v1/auth/oidc/") || r.URL.Path == "/api/v1/enroll" || strings.HasPrefix(r.URL.Path, "/api/v1/collector/") || r.URL.Path == "/collector" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie("golive_session")
		var user store.User
		tokenAuth := false
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer glv_") {
			tokenAuth = true
			user, err = a.Store.TokenUser(r.Context(), strings.TrimPrefix(auth, "Bearer "))
		} else if err == nil {
			user, err = a.Store.SessionUser(r.Context(), cookie.Value)
		}
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && user.Role == "viewer" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !tokenAuth && r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" && !strings.HasSuffix(origin, "://"+r.Host) {
				http.Error(w, "invalid request origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(api.WithUser(r.Context(), user)))
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func requestLog(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Debug("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}
