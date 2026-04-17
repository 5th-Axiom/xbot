// Package server provides an embeddable xbot server that can be used both
// as a standalone process (via main.go) and in-process (via the GUI).
package server

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"time"

	"xbot/agent"
	"xbot/bus"
	"xbot/channel"
	"xbot/config"
	"xbot/event"
	llm_pkg "xbot/llm"
	log "xbot/logger"
	"xbot/oauth"
	"xbot/oauth/providers"
	"xbot/storage"
	"xbot/storage/sqlite"
	"xbot/tools"
	"xbot/tools/feishu_mcp"
	"xbot/version"
)

// Server is an embeddable xbot server instance.
type Server struct {
	cfg     *config.Config
	backend agent.AgentBackend
	disp    *channel.Dispatcher

	// Cleanup resources
	oauthServer  *oauth.Server
	oauthManager *oauth.Manager
	sharedDB     *sqlite.DB
	tokenDB      *sqlite.DB
	webhookSrv   *event.WebhookServer
}

// New creates a new server with the given config.
// If cfg is nil, config.Load() is used.
func New(cfg *config.Config) *Server {
	if cfg == nil {
		cfg = config.Load()
	}
	return &Server{cfg: cfg}
}

// Run starts the server and blocks until ctx is cancelled.
// logWriter, if non-nil, receives a copy of all log output (for GUI embedding).
func (s *Server) Run(ctx context.Context, logWriter io.Writer) error {
	cfg := s.cfg

	// --- Logging ---
	setupCfg := log.SetupConfig{
		Level:   cfg.Log.Level,
		Format:  cfg.Log.Format,
		WorkDir: cfg.Agent.WorkDir,
		MaxAge:  7,
	}
	if logWriter != nil {
		setupCfg.ExtraWriter = logWriter
	}
	if err := log.Setup(setupCfg); err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}
	defer log.Close()

	// --- LLM ---
	llmClient, err := createLLM(cfg.LLM, llm_pkg.RetryConfig{
		Attempts: uint(cfg.Agent.LLMRetryAttempts),
		Delay:    cfg.Agent.LLMRetryDelay,
		MaxDelay: cfg.Agent.LLMRetryMaxDelay,
		Timeout:  cfg.Agent.LLMRetryTimeout,
	})
	if err != nil {
		return fmt.Errorf("create LLM: %w", err)
	}
	log.WithFields(log.Fields{"provider": cfg.LLM.Provider, "model": cfg.LLM.Model}).Info("LLM client created")

	// --- Message bus ---
	msgBus := bus.NewMessageBus()

	// --- Storage ---
	workDir := cfg.Agent.WorkDir
	xbotDir := filepath.Join(workDir, ".xbot")
	dbPath := filepath.Join(xbotDir, "xbot.db")

	if err := storage.MigrateIfNeeded(context.Background(), workDir, dbPath); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// --- OAuth ---
	oauthServer, oauthManager, feishuProvider, sharedDB, err := setupOAuth(cfg, dbPath)
	if err != nil {
		return fmt.Errorf("setup OAuth: %w", err)
	}
	s.oauthServer = oauthServer
	s.oauthManager = oauthManager
	s.sharedDB = sharedDB

	// --- Sandbox ---
	tools.InitSandbox(cfg.Sandbox, workDir)

	// --- Backend ---
	bc := agent.BackendConfig{
		Cfg:              cfg,
		LLM:              llmClient,
		Bus:              msgBus,
		DBPath:           dbPath,
		WorkDir:          workDir,
		XbotHome:         xbotDir,
		PersonaIsolation: cfg.Web.PersonaIsolation,
	}
	backend := agent.NewLocalBackend(bc.AgentConfig())
	s.backend = backend

	// --- Register tools ---
	registerTools(cfg, backend, oauthManager, feishuProvider)

	// --- Event triggers ---
	triggerSvc := sqlite.NewTriggerService(backend.MultiSession().DB())
	eventRouter := event.NewRouter(triggerSvc)
	backend.SetEventRouter(eventRouter)

	webhookBaseURL := cfg.EventWebhook.BaseURL
	if webhookBaseURL == "" {
		webhookBaseURL = fmt.Sprintf("http://%s:%d", cfg.EventWebhook.Host, cfg.EventWebhook.Port)
	}
	backend.RegisterCoreTool(tools.NewEventTriggerTool(eventRouter, webhookBaseURL))

	if cfg.EventWebhook.Enable {
		s.webhookSrv = event.NewWebhookServer(eventRouter, event.WebhookConfig{
			Host:        cfg.EventWebhook.Host,
			Port:        cfg.EventWebhook.Port,
			BaseURL:     webhookBaseURL,
			MaxBodySize: cfg.EventWebhook.MaxBodySize,
			RateLimit:   cfg.EventWebhook.RateLimit,
		})
	}

	// --- Finalize tools ---
	backend.IndexGlobalTools()
	backend.LLMFactory().SetModelTiers(cfg.LLM)

	// --- Token DB ---
	tokenDB, err := sqlite.Open(dbPath)
	if err != nil {
		log.WithError(err).Warn("Failed to open token database, runner tokens disabled")
	} else {
		tools.SetRunnerTokenDB(tokenDB.Conn())
		s.tokenDB = tokenDB
	}

	// --- Channels ---
	disp := channel.NewDispatcher(msgBus)
	s.disp = disp

	var webDB *sql.DB
	if tokenDB != nil {
		webDB = tokenDB.Conn()
	}
	feishuCh, err := registerChannels(disp, cfg, msgBus, backend, webDB, workDir)
	if err != nil {
		return fmt.Errorf("register channels: %w", err)
	}

	backend.SetDirectSend(disp.SendDirect)
	backend.SetChannelFinder(disp.GetChannel)
	wireFeishu(cfg, backend, feishuCh, webDB, disp)

	// --- Start services ---
	if oauthServer != nil {
		oauthManager.Start(ctx)
		oauthServer.SetSendFunc(func(ch, chatID, content string) error {
			_, err := disp.SendDirect(bus.OutboundMessage{Channel: ch, ChatID: chatID, Content: content})
			return err
		})
		if err := oauthServer.Start(); err != nil {
			return fmt.Errorf("start OAuth: %w", err)
		}
		log.WithFields(log.Fields{"port": cfg.OAuth.Port, "baseURL": cfg.OAuth.BaseURL}).Info("OAuth server started")
	}

	channels := disp.EnabledChannels()
	if len(channels) == 0 {
		log.Info("Starting in agent-only mode (no IM channels)")
	} else {
		log.WithField("channels", channels).Info("Channels enabled")
	}

	// Dispatcher
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("Dispatcher panicked\n" + string(debug.Stack()))
			}
		}()
		disp.Run()
	}()

	// Channels
	for name, ch := range getChannels(disp) {
		go func(n string, c channel.Channel) {
			defer func() {
				if r := recover(); r != nil {
					log.WithFields(log.Fields{"channel": n, "panic": r}).Error("Channel panicked\n" + string(debug.Stack()))
				}
			}()
			log.WithField("channel", n).Info("Starting channel...")
			if err := c.Start(); err != nil {
				log.WithError(err).WithField("channel", n).Error("Channel failed")
			}
		}(name, ch)
	}

	// Webhook
	if s.webhookSrv != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.WithField("panic", r).Error("Webhook server panicked\n" + string(debug.Stack()))
				}
			}()
			if err := s.webhookSrv.Start(); err != nil {
				log.WithError(err).Error("Webhook server failed")
			}
		}()
	}

	// Agent loop
	agentDone := make(chan struct{})
	go func() {
		defer close(agentDone)
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("Agent loop panicked\n" + string(debug.Stack()))
			}
		}()
		if err := backend.Run(ctx); err != nil && ctx.Err() == nil {
			log.WithError(err).Error("Agent loop exited with error")
		}
	}()

	log.Info("xbot started successfully")

	// Startup notify
	if cfg.StartupNotify.Channel != "" && cfg.StartupNotify.ChatID != "" {
		go sendStartupNotify(disp, cfg)
	}

	// Block until context cancelled
	<-ctx.Done()
	log.Info("Shutting down...")

	// --- Graceful shutdown ---
	s.shutdown()
	<-agentDone
	return nil
}

// shutdown performs the graceful shutdown sequence.
func (s *Server) shutdown() {
	if s.webhookSrv != nil {
		s.webhookSrv.Stop()
	}
	if s.backend != nil {
		s.backend.Close()
	}
	if sandbox := tools.GetSandbox(); sandbox != nil {
		if err := sandbox.Close(); err != nil {
			log.WithError(err).Warn("Sandbox close error")
		}
	}
	if s.oauthServer != nil {
		s.oauthServer.Shutdown(context.Background())
	}
	if s.oauthManager != nil {
		s.oauthManager.Close()
	}
	if s.sharedDB != nil {
		s.sharedDB.Close()
	}
	if s.tokenDB != nil {
		s.tokenDB.Close()
	}
	if s.disp != nil {
		s.disp.Stop()
	}
	log.Info("xbot stopped")
}

// ---------------------------------------------------------------------------
// Helper functions (extracted from main.go)
// ---------------------------------------------------------------------------

func createLLM(cfg config.LLMConfig, retryCfg llm_pkg.RetryConfig) (llm_pkg.LLM, error) {
	var inner llm_pkg.LLM
	switch cfg.Provider {
	case "openai":
		inner = llm_pkg.NewOpenAILLM(llm_pkg.OpenAIConfig{
			BaseURL:      cfg.BaseURL,
			APIKey:       cfg.APIKey,
			DefaultModel: cfg.Model,
		})
	case "anthropic":
		inner = llm_pkg.NewAnthropicLLM(llm_pkg.AnthropicConfig{
			BaseURL:      cfg.BaseURL,
			APIKey:       cfg.APIKey,
			DefaultModel: cfg.Model,
		})
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
	return llm_pkg.NewRetryLLM(inner, retryCfg), nil
}

func setupOAuth(cfg *config.Config, dbPath string) (*oauth.Server, *oauth.Manager, *providers.FeishuProvider, *sqlite.DB, error) {
	if !cfg.OAuth.Enable {
		return nil, nil, nil, nil, nil
	}
	sharedDB, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("open shared DB for OAuth: %w", err)
	}
	tokenStorage, err := oauth.NewSQLiteStorage(sharedDB)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("create OAuth storage: %w", err)
	}
	oauthManager := oauth.NewManager(tokenStorage)
	feishuProvider := providers.NewFeishuProvider(cfg.Feishu.AppID, cfg.Feishu.AppSecret, cfg.OAuth.BaseURL+"/oauth/callback")
	oauthManager.RegisterProvider(feishuProvider)
	oauthServer := oauth.NewServer(oauth.Config{Enable: true, Host: cfg.OAuth.Host, Port: cfg.OAuth.Port, BaseURL: cfg.OAuth.BaseURL}, oauthManager)
	return oauthServer, oauthManager, feishuProvider, sharedDB, nil
}

func registerTools(cfg *config.Config, backend agent.AgentBackend, oauthManager *oauth.Manager, feishuProvider *providers.FeishuProvider) {
	if cfg.OAuth.Enable && oauthManager != nil {
		oauthTool := &tools.OAuthTool{Manager: oauthManager, BaseURL: cfg.OAuth.BaseURL}
		backend.RegisterCoreTool(oauthTool)

		feishuMCP := feishu_mcp.NewFeishuMCP(oauthManager, cfg.Feishu.AppID, cfg.Feishu.AppSecret)
		if feishuProvider != nil {
			feishuMCP.SetLarkClient(feishuProvider.GetLarkClient())
		}
		backend.RegisterTool(&feishu_mcp.ListAllBitablesTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.BitableFieldsTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.BitableRecordTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.BitableListTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.BatchCreateAppTableRecordTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.WikiListSpacesTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.WikiListNodesTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.WikiGetNodeTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.WikiMoveNodeTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.WikiCreateNodeTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DocxGetContentTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DocxListBlocksTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DocxCreateTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DocxInsertBlockTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DocxGetBlockTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DocxDeleteBlocksTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DocxFindBlockTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.SearchWikiTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.UploadFileTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.ListFilesTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.AddPermissionTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.DownloadFileTool{MCP: feishuMCP})
		backend.RegisterTool(&feishu_mcp.SendFileTool{MCP: feishuMCP})
		log.Info("OAuth and Feishu MCP tools registered")
	}

	backend.RegisterCoreTool(tools.NewDownloadFileTool(cfg.Feishu.AppID, cfg.Feishu.AppSecret))
	backend.RegisterTool(tools.NewDownloadFileTool(cfg.Feishu.AppID, cfg.Feishu.AppSecret))
	backend.RegisterCoreTool(tools.NewWebSearchTool(cfg.TavilyAPIKey))

	if adminChatID := cfg.Admin.ChatID; adminChatID != "" {
		backend.RegisterCoreTool(tools.NewLogsTool(adminChatID))
		log.WithField("admin_chat_id", adminChatID).Info("Logs tool registered")
	}
}

func getChannels(disp *channel.Dispatcher) map[string]channel.Channel {
	result := make(map[string]channel.Channel)
	for _, name := range disp.EnabledChannels() {
		if ch, ok := disp.GetChannel(name); ok {
			result[name] = ch
		}
	}
	return result
}

func sendStartupNotify(disp *channel.Dispatcher, cfg *config.Config) {
	const maxWait = 10 * time.Second
	const pollInterval = 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if len(disp.EnabledChannels()) > 0 {
			time.Sleep(1 * time.Second)
			break
		}
		time.Sleep(pollInterval)
	}
	content := fmt.Sprintf("🟢 **xbot 已上线**\n- 版本：%s\n- 时间：%s\n- 模型：%s\n- 沙箱：%s\n- 记忆：%s",
		version.Info(), time.Now().Format("2006-01-02 15:04:05 MST"),
		cfg.LLM.Model, cfg.Sandbox.Mode, cfg.Agent.MemoryProvider)
	for i := 0; i < 3; i++ {
		if _, err := disp.SendDirect(bus.OutboundMessage{
			Channel: cfg.StartupNotify.Channel, ChatID: cfg.StartupNotify.ChatID, Content: content,
		}); err == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
}
