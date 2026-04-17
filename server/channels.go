package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"xbot/agent"
	"xbot/bus"
	"xbot/channel"
	"xbot/config"
	llm_pkg "xbot/llm"
	log "xbot/logger"
	"xbot/storage/sqlite"
	"xbot/tools"
)

// registerChannels creates and registers all enabled channels with the dispatcher.
func registerChannels(disp *channel.Dispatcher, cfg *config.Config, msgBus *bus.MessageBus, backend agent.AgentBackend, webDB *sql.DB, workDir string) (*channel.FeishuChannel, error) {
	var feishuCh *channel.FeishuChannel
	if cfg.Feishu.Enabled {
		feishuCh = channel.NewFeishuChannel(channel.FeishuConfig{
			AppID:             cfg.Feishu.AppID,
			AppSecret:         cfg.Feishu.AppSecret,
			EncryptKey:        cfg.Feishu.EncryptKey,
			VerificationToken: cfg.Feishu.VerificationToken,
			AllowFrom:         cfg.Feishu.AllowFrom,
		}, msgBus)
		disp.Register(feishuCh)
	}

	if cfg.QQ.Enabled {
		qqCh := channel.NewQQChannel(channel.QQConfig{
			AppID:        cfg.QQ.AppID,
			ClientSecret: cfg.QQ.ClientSecret,
			AllowFrom:    cfg.QQ.AllowFrom,
		}, msgBus)
		disp.Register(qqCh)
	}

	if cfg.NapCat.Enabled {
		napcatCh := channel.NewNapCatChannel(channel.NapCatConfig{
			WSUrl:     cfg.NapCat.WSUrl,
			Token:     cfg.NapCat.Token,
			AllowFrom: cfg.NapCat.AllowFrom,
		}, msgBus)
		disp.Register(napcatCh)
	}

	if cfg.Web.Enable {
		if webDB != nil {
			webCh := channel.NewWebChannel(channel.WebChannelConfig{
				Host:       cfg.Web.Host,
				Port:       cfg.Web.Port,
				DB:         webDB,
				AdminToken: cfg.Admin.Token,
				InviteOnly: cfg.Web.InviteOnly,
				PublicURL:  cfg.Sandbox.PublicURL,
			}, msgBus)
			if cfg.Web.StaticDir != "" {
				webCh.SetStaticDir(cfg.Web.StaticDir)
			}
			webCh.SetWorkDir(workDir)

			if cfg.OSS.Provider == "qiniu" {
				ossProvider, err := channel.NewOSSProvider(
					cfg.OSS.Provider, "",
					channel.QiniuConfig{
						AccessKey: cfg.OSS.QiniuAccessKey,
						SecretKey: cfg.OSS.QiniuSecretKey,
						Bucket:    cfg.OSS.QiniuBucket,
						Domain:    cfg.OSS.QiniuDomain,
						Region:    cfg.OSS.QiniuRegion,
					},
				)
				if err != nil {
					log.WithError(err).Error("Failed to create Qiniu OSS provider")
				} else {
					webCh.SetOSSProvider(ossProvider)
					log.Info("OSS provider configured: qiniu")
				}
			}

			webCh.SetCallbacks(buildWebCallbacks(cfg, backend))
			webCh.SetAdminCallbacks(channel.AdminCallbacks{
				MetricsGet: func() map[string]interface{} {
					snapshot := agent.GlobalMetrics.Snapshot()
					b, _ := json.Marshal(snapshot)
					var m map[string]interface{}
					json.Unmarshal(b, &m)
					return m
				},
				ChannelsGet: func() []string {
					return disp.EnabledChannels()
				},
			})

			if router, ok := tools.GetSandbox().(*tools.SandboxRouter); ok {
				if remote := router.Remote(); remote != nil {
					remote.OnRunnerStatusChange = func(userID, runnerName string, online bool) {
						webCh.PushRunnerStatus(userID, runnerName, online)
						if online {
							injectProxyLLM(userID, backend)
						} else {
							backend.ClearProxyLLM(userID)
						}
					}
					remote.OnSyncProgress = func(userID, phase, message string) {
						webCh.PushSyncProgress(userID, phase, message)
					}
				}
			}
			disp.Register(webCh)
		} else {
			log.Warn("Web channel enabled but no database available, skipping")
		}
	}

	return feishuCh, nil
}

// wireFeishu connects feishu-specific callbacks to the backend.
func wireFeishu(cfg *config.Config, backend agent.AgentBackend, feishuCh *channel.FeishuChannel, webDB *sql.DB, disp *channel.Dispatcher) {
	if feishuCh == nil {
		return
	}
	feishuCh.SetCardBuilder(backend.GetCardBuilder())
	if hook := backend.ToolHookChain().Get("approval"); hook != nil {
		if ah, ok := hook.(*tools.ApprovalHook); ok {
			feishuCh.SetApprovalHook(ah)
		}
	}
	if adminChatID := cfg.Admin.ChatID; adminChatID != "" {
		feishuCh.SetAdminChatID(adminChatID)
	}
	if webDB != nil {
		feishuCh.SetWebDB(webDB)
	}

	feishuCh.SetSettingsCallbacks(channel.SettingsCallbacks{
		LLMList: func(senderID string) ([]string, string) {
			llmClient, currentModel, _, _ := backend.LLMFactory().GetLLM(senderID)
			return llmClient.ListModels(), currentModel
		},
		LLMSet: func(senderID, model string) error {
			return backend.SetUserModel(senderID, model)
		},
		LLMGetMaxContext: func(senderID string) int {
			return backend.GetUserMaxContext(senderID)
		},
		LLMSetMaxContext: func(senderID string, maxContext int) error {
			return backend.SetUserMaxContext(senderID, maxContext)
		},
		LLMGetMaxOutputTokens: func(senderID string) int {
			return backend.GetUserMaxOutputTokens(senderID)
		},
		LLMSetMaxOutputTokens: func(senderID string, maxTokens int) error {
			return backend.SetUserMaxOutputTokens(senderID, maxTokens)
		},
		LLMGetThinkingMode: func(senderID string) string {
			return backend.GetUserThinkingMode(senderID)
		},
		LLMSetThinkingMode: func(senderID string, mode string) error {
			return backend.SetUserThinkingMode(senderID, mode)
		},
		LLMGetModelTier: func(tier string) string {
			switch tier {
			case "vanguard":
				return cfg.LLM.VanguardModel
			case "balance":
				return cfg.LLM.BalanceModel
			case "swift":
				return cfg.LLM.SwiftModel
			default:
				return ""
			}
		},
		LLMSetModelTier: func(tier, model string) error {
			switch tier {
			case "vanguard":
				cfg.LLM.VanguardModel = model
			case "balance":
				cfg.LLM.BalanceModel = model
			case "swift":
				cfg.LLM.SwiftModel = model
			default:
				return fmt.Errorf("unknown tier: %s", tier)
			}
			backend.LLMFactory().SetModelTiers(cfg.LLM)
			return config.SaveToFile(config.ConfigFilePath(), cfg)
		},
		LLMListAllModels: func() []string {
			return backend.LLMFactory().ListAllModelsForUser("")
		},
		LLMListSubscriptions: func(senderID string) ([]channel.Subscription, error) {
			subs, err := backend.LLMFactory().GetSubscriptionSvc().List(senderID)
			if err != nil {
				return nil, err
			}
			result := make([]channel.Subscription, len(subs))
			for i, s := range subs {
				result[i] = channel.Subscription{
					ID: s.ID, Name: s.Name, Provider: s.Provider,
					BaseURL: s.BaseURL, APIKey: s.APIKey,
					Model: s.Model, Active: s.IsDefault,
				}
			}
			return result, nil
		},
		LLMGetDefaultSubscription: func(senderID string) (*channel.Subscription, error) {
			sub, err := backend.LLMFactory().GetSubscriptionSvc().GetDefault(senderID)
			if err != nil || sub == nil {
				return nil, err
			}
			return &channel.Subscription{
				ID: sub.ID, Name: sub.Name, Provider: sub.Provider,
				BaseURL: sub.BaseURL, APIKey: sub.APIKey,
				Model: sub.Model, Active: sub.IsDefault,
			}, nil
		},
		LLMAddSubscription: func(senderID string, sub *channel.Subscription) error {
			svc := backend.LLMFactory().GetSubscriptionSvc()
			err := svc.Add(&sqlite.LLMSubscription{
				SenderID: senderID, Name: sub.Name, Provider: sub.Provider,
				BaseURL: sub.BaseURL, APIKey: sub.APIKey, Model: sub.Model,
			})
			if err == nil {
				backend.LLMFactory().Invalidate(senderID)
			}
			return err
		},
		LLMRemoveSubscription: func(id string) error {
			svc := backend.LLMFactory().GetSubscriptionSvc()
			sub, err := svc.Get(id)
			if err != nil {
				return err
			}
			if err := svc.Remove(id); err != nil {
				return err
			}
			backend.LLMFactory().Invalidate(sub.SenderID)
			return nil
		},
		LLMSetDefaultSubscription: func(id string) error {
			svc := backend.LLMFactory().GetSubscriptionSvc()
			if err := svc.SetDefault(id); err != nil {
				return err
			}
			sub, err := svc.Get(id)
			if err == nil && sub != nil {
				backend.LLMFactory().Invalidate(sub.SenderID)
			}
			return nil
		},
		LLMRenameSubscription: func(id, name string) error {
			return backend.LLMFactory().GetSubscriptionSvc().Rename(id, name)
		},
		ContextModeGet: func() string { return backend.GetContextMode() },
		ContextModeSet: func(mode string) error { return backend.SetContextMode(mode) },
		RegistryBrowse: func(entryType string, limit, offset int) ([]sqlite.SharedEntry, error) {
			return backend.RegistryManager().Browse(entryType, limit, offset)
		},
		RegistryInstall: func(entryType string, id int64, senderID string) error {
			return backend.RegistryManager().Install(entryType, id, senderID)
		},
		RegistryListMy: func(senderID, entryType string) ([]sqlite.SharedEntry, []string, error) {
			return backend.RegistryManager().ListMy(senderID, entryType)
		},
		RegistryPublish: func(entryType, name, senderID string) error {
			return backend.RegistryManager().Publish(entryType, name, senderID)
		},
		RegistryUnpublish: func(entryType, name, senderID string) error {
			return backend.RegistryManager().Unpublish(entryType, name, senderID)
		},
		RegistryDelete: func(entryType, name, senderID string) error {
			return backend.RegistryManager().Uninstall(entryType, name, senderID)
		},
		MetricsGet: func() string {
			return agent.GlobalMetrics.Snapshot().FormatMarkdown()
		},
		SandboxCleanupTrigger: func(senderID string) error {
			return tools.GetSandbox().ExportAndImport(senderID)
		},
		SandboxIsExporting: func(senderID string) bool {
			return tools.GetSandbox().IsExporting(senderID)
		},
		LLMGetPersonalConcurrency: func(senderID string) int {
			return backend.GetLLMConcurrency(senderID)
		},
		LLMSetPersonalConcurrency: func(senderID string, personal int) error {
			return backend.SetLLMConcurrency(senderID, personal)
		},
		RunnerConnectCmdGet: func(senderID string) string {
			token := cfg.Sandbox.AuthToken
			if token == "" {
				return ""
			}
			pubURL := cfg.Sandbox.PublicURL
			if pubURL == "" {
				pubURL = fmt.Sprintf("ws://%s:%d", cfg.Server.Host, cfg.Server.Port)
			}
			return fmt.Sprintf("./xbot-runner --server %s/ws/%s --token %s", pubURL, senderID, token)
		},
		RunnerTokenGet: func(senderID string) string {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return ""
			}
			entry := tools.NewRunnerTokenStore(db).Get(senderID)
			if entry == nil {
				return ""
			}
			return buildRunnerConnectCmd(cfg, entry)
		},
		RunnerTokenGenerate: func(senderID, mode, dockerImage, workspace string) (string, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", fmt.Errorf("remote sandbox not configured")
			}
			entry := tools.NewRunnerTokenStore(db).Generate(senderID, tools.RunnerTokenSettings{
				Mode: mode, DockerImage: dockerImage, Workspace: workspace,
			})
			if entry == nil {
				return "", fmt.Errorf("failed to generate token")
			}
			return buildRunnerConnectCmd(cfg, entry), nil
		},
		RunnerTokenRevoke: func(senderID string) error {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return fmt.Errorf("remote sandbox not configured")
			}
			tools.NewRunnerTokenStore(db).Revoke(senderID)
			return nil
		},
		RunnerList: func(senderID string) ([]tools.RunnerInfo, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return nil, fmt.Errorf("runner management not configured")
			}
			store := tools.NewRunnerTokenStore(db)
			runners, err := store.ListRunners(senderID)
			if err != nil {
				return nil, err
			}
			if sb := tools.GetSandbox(); sb != nil {
				if router, ok := sb.(*tools.SandboxRouter); ok {
					for i := range runners {
						runners[i].Online = router.IsRunnerOnline(senderID, runners[i].Name)
					}
					if router.HasDocker() {
						dockerEntry := tools.RunnerInfo{
							Name: tools.BuiltinDockerRunnerName, Mode: "docker",
							DockerImage: router.DockerImage(), Online: true,
						}
						runners = append([]tools.RunnerInfo{dockerEntry}, runners...)
					}
				}
			}
			return runners, nil
		},
		RunnerCreate: func(senderID, name, mode, dockerImage, workspace string, llm tools.RunnerLLMSettings) (string, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", fmt.Errorf("runner management not configured")
			}
			store := tools.NewRunnerTokenStore(db)
			token, _, err := store.CreateRunner(senderID, name, mode, dockerImage, workspace, llm)
			if err != nil {
				return "", err
			}
			pubURL := cfg.Sandbox.PublicURL
			if pubURL == "" {
				pubURL = fmt.Sprintf("ws://%s:%d", cfg.Server.Host, cfg.Server.Port)
			}
			cmd := fmt.Sprintf("./xbot-runner --server %s/ws/%s --token %s", pubURL, senderID, token)
			if mode == "docker" && dockerImage != "" {
				cmd += fmt.Sprintf(" --mode docker --docker-image %s", dockerImage)
			}
			if workspace != "" {
				cmd += fmt.Sprintf(" --workspace %s", workspace)
			}
			if llm.HasLLM() {
				cmd += fmt.Sprintf(" --llm-provider %s --llm-api-key %s --llm-model %s", llm.Provider, llm.APIKey, llm.Model)
				if llm.BaseURL != "" {
					cmd += fmt.Sprintf(" --llm-base-url %s", llm.BaseURL)
				}
			}
			return cmd, nil
		},
		RunnerDelete: func(senderID, name string) error {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return fmt.Errorf("runner management not configured")
			}
			if sb := tools.GetSandbox(); sb != nil {
				if router, ok := sb.(*tools.SandboxRouter); ok {
					router.DisconnectRunner(senderID, name)
				}
			}
			return tools.NewRunnerTokenStore(db).DeleteRunner(senderID, name)
		},
		RunnerGetActive: func(senderID string) (string, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", fmt.Errorf("runner management not configured")
			}
			return tools.NewRunnerTokenStore(db).GetActiveRunner(senderID)
		},
		RunnerSetActive: func(senderID, name string) error {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return fmt.Errorf("runner management not configured")
			}
			return tools.NewRunnerTokenStore(db).SetActiveRunner(senderID, name)
		},
		FeishuWebLink: func(feishuUserID, username, password string) (string, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", fmt.Errorf("web linking not enabled")
			}
			return channel.FeishuLinkUser(db, feishuUserID, username, password)
		},
		FeishuWebGetLinked: func(feishuUserID string) (string, bool) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", false
			}
			return channel.FeishuGetLinkedUser(db, feishuUserID)
		},
		FeishuWebUnlink: func(feishuUserID string) error {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return fmt.Errorf("web linking not enabled")
			}
			return channel.FeishuUnlinkUser(db, feishuUserID)
		},
		MemoryClear: func(senderID, chatID, targetType string) error {
			return backend.MultiSession().ClearMemory(context.Background(), "feishu", chatID, targetType, senderID)
		},
		MemoryGetStats: func(senderID, chatID string) map[string]string {
			return backend.MultiSession().GetMemoryStats(context.Background(), "feishu", chatID, senderID)
		},
	})

	backend.SetChannelPromptProviders(&feishuPromptAdapter{ch: feishuCh})
}

// ---------------------------------------------------------------------------
// Helpers shared with main.go (moved here)
// ---------------------------------------------------------------------------

func injectProxyLLM(userID string, backend agent.AgentBackend) {
	db := tools.GetRunnerTokenDB()
	if db == nil {
		return
	}
	store := tools.NewRunnerTokenStore(db)
	activeName, err := store.GetActiveRunner(userID)
	if err != nil || activeName == "" {
		return
	}
	runners, err := store.ListRunners(userID)
	if err != nil {
		return
	}
	for _, r := range runners {
		if r.Name == activeName {
			llm := r.LLMSettings()
			if llm.HasLLM() {
				sb := tools.GetSandbox()
				if sb == nil {
					return
				}
				router, ok := sb.(*tools.SandboxRouter)
				if !ok || router.Remote() == nil {
					return
				}
				rs := router.Remote()
				proxy := &llm_pkg.ProxyLLM{
					GenerateFunc: func(ctx context.Context, _, model string, messages []llm_pkg.ChatMessage, tls []llm_pkg.ToolDefinition, thinkingMode string) (*llm_pkg.LLMResponse, error) {
						return rs.LLMGenerate(ctx, userID, model, messages, tls, thinkingMode)
					},
					ListModelsFunc: func() []string {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						models, err := rs.LLMModels(ctx, userID)
						if err != nil {
							return nil
						}
						return models
					},
				}
				model := llm.Model
				if model == "" {
					model = backend.GetDefaultModel()
				}
				backend.SetProxyLLM(userID, proxy, model)
			} else {
				backend.ClearProxyLLM(userID)
			}
			return
		}
	}
}

func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return key[:4] + "****"
}

func buildRunnerConnectCmd(cfg *config.Config, entry *tools.RunnerTokenEntry) string {
	pubURL := cfg.Sandbox.PublicURL
	if pubURL == "" {
		pubURL = fmt.Sprintf("ws://%s:%d", cfg.Server.Host, cfg.Server.Port)
	}
	cmd := fmt.Sprintf("./xbot-runner --server %s/ws/%s --token %s", pubURL, entry.UserID, entry.Token)
	if entry.Settings.Mode == "docker" {
		cmd += " --mode docker"
		if entry.Settings.DockerImage != "" {
			cmd += fmt.Sprintf(" --docker-image %s", entry.Settings.DockerImage)
		}
	}
	if entry.Settings.Workspace != "" && entry.Settings.Workspace != "/workspace" {
		cmd += fmt.Sprintf(" --workspace %s", entry.Settings.Workspace)
	}
	return cmd
}

// feishuPromptAdapter bridges FeishuChannel to agent.ChannelPromptProvider.
type feishuPromptAdapter struct {
	ch *channel.FeishuChannel
}

func (a *feishuPromptAdapter) ChannelPromptName() string {
	return a.ch.Name()
}

func (a *feishuPromptAdapter) ChannelSystemParts(ctx context.Context, chatID, senderID string) map[string]string {
	return a.ch.ChannelSystemParts(ctx, chatID, senderID)
}

// buildWebCallbacks creates WebCallbacks (same as original main.go).
func buildWebCallbacks(cfg *config.Config, backend agent.AgentBackend) channel.WebCallbacks {
	callbacks := channel.WebCallbacks{
		RunnerTokenGet: func(senderID string) string {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return ""
			}
			entry := tools.NewRunnerTokenStore(db).Get(senderID)
			if entry == nil {
				return ""
			}
			return buildRunnerConnectCmd(cfg, entry)
		},
		RunnerTokenGenerate: func(senderID, mode, dockerImage, workspace string) (string, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", fmt.Errorf("remote sandbox not configured")
			}
			entry := tools.NewRunnerTokenStore(db).Generate(senderID, tools.RunnerTokenSettings{
				Mode: mode, DockerImage: dockerImage, Workspace: workspace,
			})
			if entry == nil {
				return "", fmt.Errorf("failed to generate token")
			}
			return buildRunnerConnectCmd(cfg, entry), nil
		},
		RunnerTokenRevoke: func(senderID string) error {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return fmt.Errorf("remote sandbox not configured")
			}
			tools.NewRunnerTokenStore(db).Revoke(senderID)
			return nil
		},
		RunnerList: func(senderID string) ([]tools.RunnerInfo, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return nil, fmt.Errorf("runner management not configured")
			}
			store := tools.NewRunnerTokenStore(db)
			runners, err := store.ListRunners(senderID)
			if err != nil {
				return nil, err
			}
			if sb := tools.GetSandbox(); sb != nil {
				if router, ok := sb.(*tools.SandboxRouter); ok {
					for i := range runners {
						runners[i].Online = router.IsRunnerOnline(senderID, runners[i].Name)
					}
				}
				if router, ok := sb.(*tools.SandboxRouter); ok && router.HasDocker() {
					runners = append([]tools.RunnerInfo{{
						Name: tools.BuiltinDockerRunnerName, Mode: "docker",
						DockerImage: router.DockerImage(), Online: true,
					}}, runners...)
				}
			}
			return runners, nil
		},
		RunnerCreate: func(senderID, name, mode, dockerImage, workspace string, llm tools.RunnerLLMSettings) (string, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", fmt.Errorf("runner management not configured")
			}
			token, _, err := tools.NewRunnerTokenStore(db).CreateRunner(senderID, name, mode, dockerImage, workspace, llm)
			if err != nil {
				return "", err
			}
			pubURL := cfg.Sandbox.PublicURL
			if pubURL == "" {
				pubURL = fmt.Sprintf("ws://%s:%d", cfg.Server.Host, cfg.Server.Port)
			}
			cmd := fmt.Sprintf("./xbot-runner --server %s/ws/%s --token %s", pubURL, senderID, token)
			if mode == "docker" && dockerImage != "" {
				cmd += fmt.Sprintf(" --mode docker --docker-image %s", dockerImage)
			}
			if workspace != "" {
				cmd += fmt.Sprintf(" --workspace %s", workspace)
			}
			if llm.HasLLM() {
				cmd += fmt.Sprintf(" --llm-provider %s --llm-api-key %s --llm-model %s", llm.Provider, llm.APIKey, llm.Model)
				if llm.BaseURL != "" {
					cmd += fmt.Sprintf(" --llm-base-url %s", llm.BaseURL)
				}
			}
			return cmd, nil
		},
		RunnerDelete: func(senderID, name string) error {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return fmt.Errorf("runner management not configured")
			}
			if sb := tools.GetSandbox(); sb != nil {
				if router, ok := sb.(*tools.SandboxRouter); ok {
					router.DisconnectRunner(senderID, name)
				}
			}
			return tools.NewRunnerTokenStore(db).DeleteRunner(senderID, name)
		},
		RunnerGetActive: func(senderID string) (string, error) {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return "", fmt.Errorf("runner management not configured")
			}
			return tools.NewRunnerTokenStore(db).GetActiveRunner(senderID)
		},
		RunnerSetActive: func(senderID, name string) error {
			db := tools.GetRunnerTokenDB()
			if db == nil {
				return fmt.Errorf("runner management not configured")
			}
			return tools.NewRunnerTokenStore(db).SetActiveRunner(senderID, name)
		},
		RegistryBrowse: func(entryType string, limit, offset int) ([]sqlite.SharedEntry, error) {
			return backend.RegistryManager().Browse(entryType, limit, offset)
		},
		RegistryInstall: func(entryType string, id int64, senderID string) error {
			return backend.RegistryManager().Install(entryType, id, senderID)
		},
		RegistryListMy: func(senderID, entryType string) ([]sqlite.SharedEntry, []string, error) {
			return backend.RegistryManager().ListMy(senderID, entryType)
		},
		RegistryUnpublish: func(entryType, name, senderID string) error {
			return backend.RegistryManager().Unpublish(entryType, name, senderID)
		},
		RegistryUninstall: func(entryType, name, senderID string) error {
			return backend.RegistryManager().Uninstall(entryType, name, senderID)
		},
		LLMList: func(senderID string) ([]string, string) {
			llmClient, currentModel, _, _ := backend.LLMFactory().GetLLM(senderID)
			return llmClient.ListModels(), currentModel
		},
		LLMSet: func(senderID, model string) error { return backend.SetUserModel(senderID, model) },
		LLMGetMaxContext: func(senderID string) int { return backend.GetUserMaxContext(senderID) },
		LLMSetMaxContext: func(senderID string, maxContext int) error {
			return backend.SetUserMaxContext(senderID, maxContext)
		},
		RegistryPublish: func(entryType, name, senderID string) error {
			return backend.RegistryManager().Publish(entryType, name, senderID)
		},
		SandboxWriteFile: func(senderID string, sandboxRelPath string, data []byte, perm os.FileMode) (string, error) {
			sandbox := tools.GetSandbox()
			if sandbox == nil {
				return "", fmt.Errorf("no sandbox available")
			}
			resolver, ok := sandbox.(tools.SandboxResolver)
			if !ok {
				return "", fmt.Errorf("sandbox does not support per-user resolution")
			}
			userSbx := resolver.SandboxForUser(senderID)
			if userSbx == nil || userSbx.Name() == "none" {
				return "", fmt.Errorf("no sandbox available for user %s", senderID)
			}
			ws := userSbx.Workspace(senderID)
			absPath := filepath.Join(ws, sandboxRelPath)
			dir := filepath.Dir(absPath)
			if err := userSbx.MkdirAll(context.Background(), dir, 0755, senderID); err != nil {
				log.WithError(err).Warn("Failed to create directory in sandbox")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := userSbx.WriteFile(ctx, absPath, data, perm, senderID); err != nil {
				return "", err
			}
			return ws, nil
		},
	}
	callbacks.RPCHandler = func(method string, params json.RawMessage, senderID string) (json.RawMessage, error) {
		return handleCLIRPC(cfg, backend, method, params, senderID)
	}
	return callbacks
}
