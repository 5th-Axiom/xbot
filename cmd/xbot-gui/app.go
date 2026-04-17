//go:build gui

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xbot/config"
	"xbot/server"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const logBufCapacity = 10000
const (
	guiSessionFileName = "gui-session.json"
	a2aBaseURL         = "https://5th-axiom.com/api/v2"
	a2aRequestTimeout  = 20 * time.Second
	// devBypass: set to true to skip real SMS/login API and accept code "123456".
	devBypass     = false
	devBypassCode = "123456"
)

// guiSession persists the authenticated user's JWT and identity.
type guiSession struct {
	Token   string `json:"token"`
	Phone   string `json:"phone"`
	UserUID string `json:"user_uid,omitempty"`
	AgentUID string `json:"agent_uid,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
}

// App is the main GUI backend, exposed to the frontend via Wails bindings.
type App struct {
	ctx context.Context

	srvMu      sync.RWMutex
	authMu     sync.Mutex
	srvCancel  context.CancelFunc // cancels the in-process server
	srvDone    chan struct{}       // closed when server goroutine exits
	logBuf     *RingBuffer
	startTime  time.Time
	serverPort int
	adminToken string
	running    bool
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{
		logBuf: NewRingBuffer(logBufCapacity),
	}
}

// startup is called when the Wails app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if _, err := config.EnsureGUIConfig(); err != nil {
		println("Error ensuring GUI config:", err.Error())
	}
}

// shutdown is called when the Wails app is closing.
func (a *App) shutdown(_ context.Context) {
	_ = a.StopServer()
}

// ---------------------------------------------------------------------------
// Config management (works without server running)
// ---------------------------------------------------------------------------

// LoadConfig reads the current configuration from disk.
func (a *App) LoadConfig() (*config.Config, error) {
	cfg := config.LoadFromFile(config.ConfigFilePath())
	if cfg == nil {
		return &config.Config{}, nil
	}
	return cfg, nil
}

// SaveConfig writes configuration to disk.
func (a *App) SaveConfig(cfg *config.Config) error {
	return config.SaveToFile(config.ConfigFilePath(), cfg)
}

// GetConfigPath returns the path to the config file.
func (a *App) GetConfigPath() string {
	return config.ConfigFilePath()
}

// GetGUIConfigPath returns the path to the GUI-specific config file.
func (a *App) GetGUIConfigPath() string {
	return config.GUIConfigFilePath()
}

// GetAuthStatus reports whether the desktop GUI has a valid login session.
func (a *App) GetAuthStatus() (map[string]interface{}, error) {
	a.authMu.Lock()
	defer a.authMu.Unlock()

	session, err := a.readDesktopSession()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	authenticated := session != nil && strings.TrimSpace(session.Token) != ""

	result := map[string]interface{}{
		"authenticated": authenticated,
	}
	if authenticated {
		result["phone"] = session.Phone
		result["user_uid"] = session.UserUID
		result["agent_uid"] = session.AgentUID
		result["agent_name"] = session.AgentName
	}
	return result, nil
}

// Logout clears the local session and notifies the A2A server.
func (a *App) Logout() error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if session, err := a.readDesktopSession(); err == nil && session != nil && session.Token != "" {
		_, _ = a.a2aRequest("POST", "/auth/logout", nil, session.Token)
	}
	return a.clearDesktopSession()
}

// SendLoginCode sends a verification code to the given phone number.
func (a *App) SendLoginCode(countryCode, phoneNumber string) (map[string]interface{}, error) {
	a.authMu.Lock()
	defer a.authMu.Unlock()

	fullPhone, err := buildFullPhone(countryCode, phoneNumber)
	if err != nil {
		return nil, err
	}

	if devBypass {
		return map[string]interface{}{
			"message": fmt.Sprintf("Dev mode: use code %s for %s.", devBypassCode, maskFullPhone(fullPhone)),
		}, nil
	}

	if _, err := a.a2aRequest("POST", "/auth/send-code", map[string]string{"phone": fullPhone}, ""); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"message": fmt.Sprintf("Verification code sent to %s.", maskFullPhone(fullPhone)),
	}, nil
}

// Login verifies the code and persists the JWT session.
func (a *App) Login(countryCode, phoneNumber, code string) error {
	a.authMu.Lock()
	defer a.authMu.Unlock()

	fullPhone, err := buildFullPhone(countryCode, phoneNumber)
	if err != nil {
		return err
	}
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return errors.New("verification code is required")
	}

	if devBypass {
		if trimmedCode != devBypassCode {
			return errors.New("invalid verification code")
		}
		devToken, _ := generateAdminToken()
		return a.writeDesktopSession(guiSession{
			Token: "dev-" + devToken, Phone: fullPhone,
			UserUID: "dev-user", AgentUID: "dev-agent",
		})
	}

	data, err := a.a2aRequest("POST", "/auth/login", map[string]string{
		"phone": fullPhone, "code": trimmedCode,
	}, "")
	if err != nil {
		return err
	}
	var res struct {
		Token string `json:"token"`
		User  struct {
			UID   string `json:"uid"`
			Phone string `json:"phone"`
			Agent struct {
				UID  string `json:"uid"`
				Name string `json:"name"`
			} `json:"agent"`
		} `json:"user"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("parse login response: %w", err)
	}
	if res.Token == "" {
		return errors.New("server did not return a token")
	}
	return a.writeDesktopSession(guiSession{
		Token:     res.Token,
		Phone:     fullPhone,
		UserUID:   res.User.UID,
		AgentUID:  res.User.Agent.UID,
		AgentName: res.User.Agent.Name,
	})
}

// RefreshToken silently refreshes the JWT. Returns the new auth status.
// Only clears the session on definitive auth failures (401/403, token revoked).
// Transient errors (network, 5xx) are treated as retryable — the existing
// session is preserved and the user stays authenticated.
func (a *App) RefreshToken() (map[string]interface{}, error) {
	a.authMu.Lock()
	defer a.authMu.Unlock()

	session, err := a.readDesktopSession()
	if err != nil || session == nil || session.Token == "" {
		_ = a.clearDesktopSession()
		return map[string]interface{}{"authenticated": false}, nil
	}

	if devBypass {
		return map[string]interface{}{"authenticated": true}, nil
	}

	data, err := a.a2aRequest("POST", "/auth/refresh", nil, session.Token)
	if err != nil {
		if isAuthError(err) {
			_ = a.clearDesktopSession()
			return map[string]interface{}{"authenticated": false}, nil
		}
		// Transient error: keep session, stay authenticated
		return map[string]interface{}{"authenticated": true}, nil
	}
	var res struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &res); err != nil || res.Token == "" {
		// Malformed response — treat as transient, keep session
		return map[string]interface{}{"authenticated": true}, nil
	}
	session.Token = res.Token
	_ = a.writeDesktopSession(*session)
	return map[string]interface{}{"authenticated": true}, nil
}

// ---------------------------------------------------------------------------
// Agent Profile (A2A API)
// ---------------------------------------------------------------------------

// AgentProfile represents the agent's public profile on the A2A platform.
type AgentProfile struct {
	UID           string   `json:"uid,omitempty"`
	Name          string   `json:"name"`
	Bio           string   `json:"bio"`
	Tags          []string `json:"tags"`
	Goals         string   `json:"goals"`
	RecentContext string   `json:"recent_context"`
	LookingFor    string   `json:"looking_for"`
	City          string   `json:"city"`
	Status        string   `json:"status,omitempty"`
}

// GetProfile fetches the current agent profile from the A2A API.
func (a *App) GetProfile() (*AgentProfile, error) {
	token, err := a.getSessionToken()
	if err != nil {
		return nil, err
	}
	data, err := a.a2aRequest("GET", "/agents/me", nil, token)
	if err != nil {
		return nil, err
	}
	var profile AgentProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	return &profile, nil
}

// UpdateProfile patches the agent profile on the A2A API.
func (a *App) UpdateProfile(profile *AgentProfile) error {
	token, err := a.getSessionToken()
	if err != nil {
		return err
	}
	_, err = a.a2aRequest("PATCH", "/agents/me", profile, token)
	return err
}

// getSessionToken reads the JWT from the persisted session.
func (a *App) getSessionToken() (string, error) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	session, err := a.readDesktopSession()
	if err != nil {
		return "", errors.New("not logged in")
	}
	if session.Token == "" {
		return "", errors.New("not logged in")
	}
	return session.Token, nil
}

// ---------------------------------------------------------------------------
// Process management
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// LLM config (currently mock, will be fetched from A2A API later)
// ---------------------------------------------------------------------------

// LLMConfigSpec is the shape returned to the frontend for display/debugging.
type LLMConfigSpec struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

// loadLLMConfig reads LLM settings from gui-config.json, then falls back to
// environment variables. No secrets are hard-coded in source.
func loadLLMConfig() LLMConfigSpec {
	cfg := config.LoadGUIConfig()
	spec := LLMConfigSpec{
		Provider: cfg.LLM.Provider,
		BaseURL:  cfg.LLM.BaseURL,
		APIKey:   cfg.LLM.APIKey,
		Model:    cfg.LLM.Model,
	}
	// Env overrides
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		spec.Provider = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		spec.BaseURL = v
	}
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		spec.APIKey = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		spec.Model = v
	}
	// Defaults
	if spec.Provider == "" {
		spec.Provider = "openai"
	}
	return spec
}

// GetLLMConfig returns the LLM configuration (mock for now, real API later).
// API key is masked for display.
func (a *App) GetLLMConfig() (LLMConfigSpec, error) {
	spec := loadLLMConfig()
	spec.APIKey = maskAPIKey(spec.APIKey)
	return spec, nil
}

// GetServerInfo returns connection info for the embedded server (port + admin token).
// Used by the frontend chat page to open a WebSocket to the local server.
func (a *App) GetServerInfo() map[string]interface{} {
	a.srvMu.RLock()
	defer a.srvMu.RUnlock()

	userID := "admin"
	if session, err := a.readDesktopSession(); err == nil && session != nil {
		if session.UserUID != "" {
			userID = session.UserUID
		}
	}

	return map[string]interface{}{
		"running":     a.running,
		"port":        a.serverPort,
		"admin_token": a.adminToken,
		"user_id":     userID,
	}
}

// maskAPIKey returns a UI-safe representation of an API key: first 4 chars + "****".
func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}

// ---------------------------------------------------------------------------
// Process management
// ---------------------------------------------------------------------------

// StartServer starts the xbot server in-process (no subprocess).
// It auto-applies the mock LLM config, enables the Web channel, and ensures
// an admin token is configured so the GUI can connect to the local WS.
func (a *App) StartServer() error {
	// --- Phase 1: under the lock, reserve state and start the goroutine ---
	a.srvMu.Lock()
	if a.running {
		a.srvMu.Unlock()
		return fmt.Errorf("server is already running")
	}

	// Load config, then patch it with GUI-managed values.
	cfg := config.Load()
	applyGUIOverrides(cfg)

	a.serverPort = cfg.Web.Port
	if a.serverPort == 0 {
		a.serverPort = 8082
	}
	a.adminToken = cfg.Admin.Token
	a.logBuf = NewRingBuffer(logBufCapacity)

	// Pipe server logs into the ring buffer + frontend events.
	pr, pw := io.Pipe()
	go a.streamLogs(pr)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	a.srvCancel = cancel
	a.srvDone = done
	a.running = true
	a.startTime = time.Now()

	srv := server.New(cfg)
	// Release the lock BEFORE waiting on `done` — otherwise the goroutine's
	// own Lock() (to flip running=false on exit) would block forever and
	// deadlock the whole startup sequence.
	a.srvMu.Unlock()

	go func() {
		defer close(done)
		err := srv.Run(ctx, pw)
		pw.Close()

		a.srvMu.Lock()
		a.running = false
		a.srvCancel = nil
		a.srvDone = nil
		a.srvMu.Unlock()

		if err != nil && ctx.Err() == nil {
			a.logBuf.Add(fmt.Sprintf(`{"level":"error","msg":"server exited: %v"}`, err))
		}
		wailsRuntime.EventsEmit(a.ctx, "server-status", "stopped")
	}()

	// --- Phase 2: lock-free wait for immediate startup failures ---
	select {
	case <-done:
		return fmt.Errorf("server exited during startup%s", formatRecentLogs(a.logBuf.Last(10)))
	case <-time.After(1500 * time.Millisecond):
		// Still running — good
	}

	wailsRuntime.EventsEmit(a.ctx, "server-status", "running")
	return nil
}

// StopServer gracefully stops the in-process server.
func (a *App) StopServer() error {
	a.srvMu.Lock()
	cancel := a.srvCancel
	done := a.srvDone
	a.srvMu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()

	// Wait up to 15 seconds for clean shutdown
	if done != nil {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
		}
	}
	return nil
}

// RestartServer stops and restarts the server.
func (a *App) RestartServer() error {
	if err := a.StopServer(); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	return a.StartServer()
}

// IsRunning returns true if the server is running in-process.
func (a *App) IsRunning() bool {
	a.srvMu.RLock()
	defer a.srvMu.RUnlock()
	return a.running
}

// GetUptime returns the server uptime in seconds (0 if not running).
func (a *App) GetUptime() int64 {
	if !a.IsRunning() {
		return 0
	}
	return int64(time.Since(a.startTime).Seconds())
}

// ---------------------------------------------------------------------------
// Log management
// ---------------------------------------------------------------------------

// GetLogs returns the last n log lines.
func (a *App) GetLogs(count int) []string {
	return a.logBuf.Last(count)
}

func (a *App) streamLogs(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		a.logBuf.Add(line)
		wailsRuntime.EventsEmit(a.ctx, "log-line", line)
	}
}

// ---------------------------------------------------------------------------
// Admin API queries (require running server)
// ---------------------------------------------------------------------------

// GetHealth fetches server health from the admin API.
func (a *App) GetHealth() (map[string]interface{}, error) {
	return a.adminGet("/api/admin/health")
}

// GetMetrics fetches agent metrics from the admin API.
func (a *App) GetMetrics() (map[string]interface{}, error) {
	return a.adminGet("/api/admin/metrics")
}

// ListUsers fetches web users from the admin API.
func (a *App) ListUsers() ([]map[string]interface{}, error) {
	data, err := a.adminRequest("GET", "/api/admin/users", nil)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}

// CreateUser creates a new web user via the admin API.
func (a *App) CreateUser(username string) (map[string]interface{}, error) {
	body := map[string]string{"username": username}
	data, err := a.adminRequest("POST", "/api/admin/users", body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}

// DeleteUser deletes a web user via the admin API.
func (a *App) DeleteUser(username string) error {
	body := map[string]string{"username": username}
	_, err := a.adminRequest("DELETE", "/api/admin/users/delete", body)
	return err
}

// GetChannels fetches enabled channels from the admin API.
func (a *App) GetChannels() ([]string, error) {
	data, err := a.adminRequest("GET", "/api/admin/channels", nil)
	if err != nil {
		return nil, err
	}
	var result []string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (a *App) adminGet(path string) (map[string]interface{}, error) {
	data, err := a.adminRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}

func (a *App) adminRequest(method, path string, body interface{}) ([]byte, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("server is not running")
	}
	if a.adminToken == "" {
		return nil, fmt.Errorf("admin token not configured (set admin.token in config)")
	}

	url := fmt.Sprintf("http://127.0.0.1:%d%s", a.serverPort, path)

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(b))
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.adminToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// (subprocess helpers removed — server now runs in-process)

func formatRecentLogs(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return "\nRecent logs:\n" + strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Session file management
// ---------------------------------------------------------------------------

func (a *App) desktopSessionPath() string {
	return filepath.Join(config.XbotHome(), guiSessionFileName)
}

func (a *App) readDesktopSession() (*guiSession, error) {
	data, err := os.ReadFile(a.desktopSessionPath())
	if err != nil {
		return nil, err
	}
	var session guiSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parse desktop session: %w", err)
	}
	return &session, nil
}

func (a *App) writeDesktopSession(session guiSession) error {
	path := a.desktopSessionPath()
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal desktop session: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write desktop session: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("persist desktop session: %w", err)
	}
	return nil
}

func (a *App) clearDesktopSession() error {
	err := os.Remove(a.desktopSessionPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// A2A API client
// ---------------------------------------------------------------------------

// a2aResponse is the unified response envelope used by the A2A API.
type a2aResponse struct {
	Code    int             `json:"c"`
	Message string          `json:"m,omitempty"`
	Data    json.RawMessage `json:"d,omitempty"`
}

// a2aError is returned by a2aRequest for API-level failures (non-zero code or HTTP 4xx/5xx).
type a2aError struct {
	HTTPStatus int
	APICode    int
	Message    string
}

func (e *a2aError) Error() string { return e.Message }

// isAuthError returns true if the error indicates an invalid/expired token
// (as opposed to a transient network or server error).
func isAuthError(err error) bool {
	var ae *a2aError
	if errors.As(err, &ae) {
		return ae.HTTPStatus == 401 || ae.HTTPStatus == 403 ||
			(ae.APICode >= 40000 && ae.APICode < 41000)
	}
	return false
}

// a2aRequest calls the A2A REST API. If token is empty, no Authorization header is sent.
// Returns the unmarshalled `d` field, or an error if the server returned a non-zero `c`.
func (a *App) a2aRequest(method, path string, body interface{}, token string) (json.RawMessage, error) {
	url := a2aBaseURL + path

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(b))
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: a2aRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var envelope a2aResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		if resp.StatusCode >= 400 {
			return nil, &a2aError{HTTPStatus: resp.StatusCode, Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(data))}
		}
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Code != 0 {
		msg := envelope.Message
		if msg == "" {
			msg = fmt.Sprintf("error code %d", envelope.Code)
		}
		return nil, &a2aError{HTTPStatus: resp.StatusCode, APICode: envelope.Code, Message: msg}
	}
	return envelope.Data, nil
}

// ---------------------------------------------------------------------------
// Server config overrides (applied before starting the embedded server)
// ---------------------------------------------------------------------------

// applyGUIOverrides mutates cfg in place to ensure the embedded server runs
// with: mock LLM config, Web channel on, admin token set, local sandbox, flat memory.
// The on-disk config.json is NOT modified — changes are runtime-only.
func applyGUIOverrides(cfg *config.Config) {
	if cfg == nil {
		return
	}

	// Mock LLM config (will be replaced by A2A API fetch later)
	llmSpec := loadLLMConfig()
	cfg.LLM.Provider = llmSpec.Provider
	cfg.LLM.BaseURL = llmSpec.BaseURL
	cfg.LLM.APIKey = llmSpec.APIKey
	cfg.LLM.Model = llmSpec.Model
	// Ensure subscriptions are consistent with cfg.LLM so the factory picks it up
	cfg.Subscriptions = []config.SubscriptionConfig{{
		ID:       "gui-default",
		Name:     "GUI Default",
		Provider: llmSpec.Provider,
		BaseURL:  llmSpec.BaseURL,
		APIKey:   llmSpec.APIKey,
		Model:    llmSpec.Model,
		Active:   true,
	}}

	// Enable Web channel so the GUI can chat via the local WS endpoint
	cfg.Web.Enable = true
	if cfg.Web.Host == "" {
		cfg.Web.Host = "127.0.0.1"
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8082
	}

	// Generate and persist an admin token if none exists so the GUI can
	// authenticate against /ws and /api/admin/*.
	if strings.TrimSpace(cfg.Admin.Token) == "" {
		if tok, err := generateAdminToken(); err == nil {
			cfg.Admin.Token = tok
		}
	}

	// Sensible defaults for desktop single-user mode
	if cfg.Agent.MemoryProvider == "" {
		cfg.Agent.MemoryProvider = "flat"
	}
	if cfg.Sandbox.Mode == "" {
		cfg.Sandbox.Mode = "none"
	}
	// GUI: always use an absolute WorkDir. Relative "." is unsafe because
	// macOS .app bundles launched from Finder have CWD=/ (unwritable).
	if cfg.Agent.WorkDir == "" || cfg.Agent.WorkDir == "." {
		cfg.Agent.WorkDir = config.XbotHome()
	} else if !filepath.IsAbs(cfg.Agent.WorkDir) {
		cfg.Agent.WorkDir, _ = filepath.Abs(cfg.Agent.WorkDir)
	}
}

// generateAdminToken returns a random 32-hex-char admin token.
func generateAdminToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ---------------------------------------------------------------------------
// Phone helpers
// ---------------------------------------------------------------------------

// buildFullPhone combines country code and phone number into "+8613800138000".
func buildFullPhone(countryCode, phoneNumber string) (string, error) {
	countryCode = strings.TrimSpace(countryCode)
	digits := normalizePhoneNumber(phoneNumber)
	if countryCode == "" {
		return "", errors.New("country code is required")
	}
	if !strings.HasPrefix(countryCode, "+") {
		return "", errors.New("country code must start with +")
	}
	for _, r := range countryCode[1:] {
		if r < '0' || r > '9' {
			return "", errors.New("country code must contain digits only")
		}
	}
	if digits == "" {
		return "", errors.New("phone number is required")
	}
	return countryCode + digits, nil
}

func normalizePhoneNumber(phoneNumber string) string {
	var digits strings.Builder
	for _, r := range phoneNumber {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}

// maskFullPhone masks middle digits of a full phone like "+8613812345678" → "+86138****5678".
func maskFullPhone(fullPhone string) string {
	if len(fullPhone) <= 7 {
		return fullPhone
	}
	head := 6
	tail := 4
	if head+tail >= len(fullPhone) {
		return fullPhone
	}
	return fullPhone[:head] + strings.Repeat("*", len(fullPhone)-head-tail) + fullPhone[len(fullPhone)-tail:]
}
