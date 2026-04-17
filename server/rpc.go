package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"xbot/agent"
	"xbot/channel"
	"xbot/config"
	"xbot/storage/sqlite"
)

// handleCLIRPC dispatches RPC requests from CLI RemoteBackend clients.
func handleCLIRPC(cfg *config.Config, backend agent.AgentBackend, method string, params json.RawMessage, senderID string) (json.RawMessage, error) {
	switch method {
	case "get_context_mode":
		return json.Marshal(backend.GetContextMode())
	case "set_context_mode":
		var p struct{ Mode string `json:"mode"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		return nil, backend.SetContextMode(p.Mode)
	case "get_settings":
		var p struct{ Namespace string `json:"namespace"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.SettingsService() == nil { return nil, fmt.Errorf("settings service not available") }
		result, err := backend.SettingsService().GetSettings(p.Namespace, senderID)
		if err != nil { return nil, err }
		return json.Marshal(result)
	case "set_setting":
		var p struct{ Namespace, Key, Value string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.SettingsService() == nil { return nil, fmt.Errorf("settings service not available") }
		return nil, backend.SettingsService().SetSetting(p.Namespace, senderID, p.Key, p.Value)
	case "set_max_iterations":
		var p struct{ N int `json:"n"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		backend.SetMaxIterations(p.N); return nil, nil
	case "set_max_concurrency":
		var p struct{ N int `json:"n"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		backend.SetMaxConcurrency(p.N); return nil, nil
	case "set_max_context_tokens":
		var p struct{ N int `json:"n"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		backend.SetMaxContextTokens(p.N); return nil, nil
	case "get_default_model":
		return json.Marshal(backend.GetDefaultModel())
	case "set_user_model":
		var p struct{ Model string `json:"model"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		return nil, backend.SetUserModel(senderID, p.Model)
	case "get_user_max_context":
		return json.Marshal(backend.GetUserMaxContext(senderID))
	case "set_user_max_context":
		var p struct{ MaxContext int `json:"max_context"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		return nil, backend.SetUserMaxContext(senderID, p.MaxContext)
	case "get_user_max_output_tokens":
		return json.Marshal(backend.GetUserMaxOutputTokens(senderID))
	case "set_user_max_output_tokens":
		var p struct{ MaxTokens int `json:"max_tokens"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		return nil, backend.SetUserMaxOutputTokens(senderID, p.MaxTokens)
	case "get_user_thinking_mode":
		return json.Marshal(backend.GetUserThinkingMode(senderID))
	case "set_user_thinking_mode":
		var p struct{ Mode string `json:"mode"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		return nil, backend.SetUserThinkingMode(senderID, p.Mode)
	case "get_llm_concurrency":
		return json.Marshal(backend.GetLLMConcurrency(senderID))
	case "set_llm_concurrency":
		var p struct{ Personal int `json:"personal"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		return nil, backend.SetLLMConcurrency(senderID, p.Personal)
	case "set_default_thinking_mode":
		var p struct{ Mode string `json:"mode"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		backend.LLMFactory().SetDefaultThinkingMode(p.Mode); return nil, nil
	case "list_models":
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		return json.Marshal(backend.LLMFactory().ListModels())
	case "list_all_models":
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		return json.Marshal(backend.LLMFactory().ListAllModelsForUser(senderID))
	case "set_model_tiers":
		if senderID != "admin" { return nil, fmt.Errorf("admin only") }
		var llmCfg config.LLMConfig
		if err := json.Unmarshal(params, &llmCfg); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		backend.LLMFactory().SetModelTiers(llmCfg); return nil, nil
	case "set_proxy_llm":
		var p struct{ Model string `json:"model"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() != nil { backend.LLMFactory().SwitchModel(senderID, p.Model) }
		return nil, nil
	case "clear_proxy_llm":
		backend.ClearProxyLLM(senderID); return nil, nil
	case "clear_memory":
		var p struct{ Channel, ChatID, TargetType string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.MultiSession() == nil { return nil, fmt.Errorf("multi-session not available") }
		if senderID != "admin" && p.ChatID != "" && p.ChatID != senderID { return nil, fmt.Errorf("access denied") }
		if p.ChatID == "" { p.ChatID = senderID }
		return nil, backend.MultiSession().ClearMemory(context.Background(), p.Channel, p.ChatID, p.TargetType, senderID)
	case "get_memory_stats":
		var p struct{ Channel, ChatID string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.MultiSession() == nil { return nil, fmt.Errorf("multi-session not available") }
		if senderID != "admin" && p.ChatID != "" && p.ChatID != senderID { return nil, fmt.Errorf("access denied") }
		if p.ChatID == "" { p.ChatID = senderID }
		return json.Marshal(backend.MultiSession().GetMemoryStats(context.Background(), p.Channel, p.ChatID, senderID))
	case "get_user_token_usage":
		if backend.MultiSession() == nil { return nil, fmt.Errorf("multi-session not available") }
		usage, err := backend.MultiSession().GetUserTokenUsage(senderID)
		if err != nil { return nil, err }
		return json.Marshal(usage)
	case "get_daily_token_usage":
		var p struct{ Days int `json:"days"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.MultiSession() == nil { return nil, fmt.Errorf("multi-session not available") }
		daily, err := backend.MultiSession().GetDailyTokenUsage(senderID, p.Days)
		if err != nil { return nil, err }
		return json.Marshal(daily)
	case "count_interactive_sessions":
		var p struct{ Channel, ChatID string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if senderID != "admin" && p.ChatID != "" && p.ChatID != senderID { return nil, fmt.Errorf("access denied") }
		if p.ChatID == "" { p.ChatID = senderID }
		return json.Marshal(backend.CountInteractiveSessions(p.Channel, p.ChatID))
	case "list_interactive_sessions":
		var p struct{ Channel, ChatID string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if senderID != "admin" && p.ChatID != "" && p.ChatID != senderID { return nil, fmt.Errorf("access denied") }
		if p.ChatID == "" { p.ChatID = senderID }
		return json.Marshal(backend.ListInteractiveSessions(p.Channel, p.ChatID))
	case "inspect_interactive_session":
		var p struct{ Role, Channel, ChatID, Instance string; TailCount int `json:"tail_count"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if senderID != "admin" && p.ChatID != "" && p.ChatID != senderID { return nil, fmt.Errorf("access denied") }
		if p.ChatID == "" { p.ChatID = senderID }
		result, err := backend.InspectInteractiveSession(context.Background(), p.Role, p.Channel, p.ChatID, p.Instance, p.TailCount)
		if err != nil { return nil, err }
		return json.Marshal(result)
	case "get_bg_task_count":
		var p struct{ SessionKey string `json:"session_key"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.BgTaskManager() == nil { return json.Marshal(0) }
		return json.Marshal(len(backend.BgTaskManager().List(p.SessionKey)))
	case "get_history":
		var p struct{ Channel, ChatID string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if p.Channel == "" { p.Channel = "web" }
		if p.ChatID == "" { p.ChatID = senderID }
		if senderID != "admin" && p.ChatID != senderID { return nil, fmt.Errorf("access denied") }
		history, err := backend.GetHistory(p.Channel, p.ChatID)
		if err != nil { return nil, err }
		return json.Marshal(history)
	case "trim_history":
		var p struct{ Channel, ChatID, Cutoff string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if p.Channel == "" { p.Channel = "web" }
		if p.ChatID == "" { p.ChatID = senderID }
		if senderID != "admin" && p.ChatID != senderID { return nil, fmt.Errorf("access denied") }
		var cutoff time.Time
		if p.Cutoff != "" {
			var err error
			cutoff, err = time.Parse(time.RFC3339, p.Cutoff)
			if err != nil { return nil, fmt.Errorf("invalid cutoff: %w", err) }
		}
		return nil, backend.TrimHistory(p.Channel, p.ChatID, cutoff)
	case "list_subscriptions":
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return json.Marshal([]channel.Subscription{}) }
		subs, err := svc.List(senderID)
		if err != nil { return nil, err }
		result := make([]channel.Subscription, len(subs))
		for i, s := range subs {
			result[i] = channel.Subscription{ID: s.ID, Name: s.Name, Provider: s.Provider, BaseURL: s.BaseURL, APIKey: maskAPIKey(s.APIKey), Model: s.Model, Active: s.IsDefault}
		}
		return json.Marshal(result)
	case "get_default_subscription":
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return nil, nil }
		sub, err := svc.GetDefault(senderID)
		if err != nil || sub == nil { return nil, err }
		return json.Marshal(channel.Subscription{ID: sub.ID, Name: sub.Name, Provider: sub.Provider, BaseURL: sub.BaseURL, APIKey: maskAPIKey(sub.APIKey), Model: sub.Model, Active: sub.IsDefault})
	case "add_subscription":
		var p struct{ Sub sqlite.LLMSubscription `json:"sub"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return nil, fmt.Errorf("subscription service not available") }
		p.Sub.SenderID = senderID
		return nil, svc.Add(&p.Sub)
	case "update_subscription":
		var p struct{ ID string `json:"id"`; Sub sqlite.LLMSubscription `json:"sub"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return nil, fmt.Errorf("subscription service not available") }
		existing, err := svc.Get(p.ID)
		if err != nil { return nil, err }
		if senderID != "admin" && existing.SenderID != senderID { return nil, fmt.Errorf("subscription not found") }
		p.Sub.ID = p.ID; p.Sub.SenderID = existing.SenderID
		if strings.HasSuffix(p.Sub.APIKey, "****") { p.Sub.APIKey = existing.APIKey }
		if err := svc.Update(&p.Sub); err != nil { return nil, err }
		backend.LLMFactory().Invalidate(existing.SenderID); return nil, nil
	case "remove_subscription":
		var p struct{ ID string `json:"id"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return nil, fmt.Errorf("subscription service not available") }
		sub, err := svc.Get(p.ID)
		if err != nil { return nil, err }
		if senderID != "admin" && sub.SenderID != senderID { return nil, fmt.Errorf("subscription not found") }
		if err := svc.Remove(p.ID); err != nil { return nil, err }
		backend.LLMFactory().Invalidate(sub.SenderID); return nil, nil
	case "set_default_subscription":
		var p struct{ ID string `json:"id"` }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return nil, fmt.Errorf("subscription service not available") }
		sub, err := svc.Get(p.ID)
		if err != nil { return nil, err }
		if senderID != "admin" && sub.SenderID != senderID { return nil, fmt.Errorf("subscription not found") }
		if err := svc.SetDefault(p.ID); err != nil { return nil, err }
		backend.LLMFactory().Invalidate(sub.SenderID); return nil, nil
	case "rename_subscription":
		var p struct{ ID, Name string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return nil, fmt.Errorf("subscription service not available") }
		sub, err := svc.Get(p.ID)
		if err != nil { return nil, err }
		if senderID != "admin" && sub.SenderID != senderID { return nil, fmt.Errorf("subscription not found") }
		return nil, svc.Rename(p.ID, p.Name)
	case "set_subscription_model":
		var p struct{ ID, Model string }
		if err := json.Unmarshal(params, &p); err != nil { return nil, err }
		if backend.LLMFactory() == nil { return nil, fmt.Errorf("LLM factory not available") }
		svc := backend.LLMFactory().GetSubscriptionSvc()
		if svc == nil { return nil, fmt.Errorf("subscription service not available") }
		sub, err := svc.Get(p.ID)
		if err != nil { return nil, err }
		if senderID != "admin" && sub.SenderID != senderID { return nil, fmt.Errorf("subscription not found") }
		return nil, svc.SetModel(p.ID, p.Model)
	case "reset_token_state":
		backend.ResetTokenState(); return nil, nil
	default:
		return nil, fmt.Errorf("unknown RPC method: %s", method)
	}
}
