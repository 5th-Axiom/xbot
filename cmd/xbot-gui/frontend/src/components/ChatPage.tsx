import { useCallback, useEffect, useRef, useState } from "react";
import {
  GetLLMConfig,
  GetServerInfo,
  IsRunning,
  StartServer,
} from "../../wailsjs/go/main/App";

interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  pending?: boolean;
  kind?: "error" | "info";
}

const errorPatterns = [
  /LLM 服务调用失败/,
  /处理消息时发生错误/,
  /All attempts fail/i,
  /Arrearage/i,
  /\b4\d{2}\s+(?:Bad Request|Unauthorized|Forbidden|Not Found)/i,
  /\b5\d{2}\s+(?:Internal|Bad Gateway|Service Unavailable)/i,
  /LLM generate failed/i,
  /rpc error/i,
  /connection refused/i,
  /timeout/i,
  /Access denied/i,
  /stream completion/i,
];

function isErrorLike(content: string): boolean {
  return errorPatterns.some((p) => p.test(content));
}

interface ServerInfo {
  running: boolean;
  port: number;
  admin_token: string;
}

interface LLMInfo {
  provider: string;
  base_url: string;
  api_key: string;
  model: string;
}

function genID(): string {
  return Math.random().toString(36).slice(2, 10);
}

interface ChatPageProps {
  onOpenProfile: () => void;
}

export default function ChatPage({ onOpenProfile }: ChatPageProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [connected, setConnected] = useState(false);
  const [status, setStatus] = useState<string>("Preparing the workspace...");
  const [llm, setLlm] = useState<LLMInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [thinking, setThinking] = useState<string>("");

  const wsRef = useRef<WebSocket | null>(null);
  const assistantBufRef = useRef<{ id: string; content: string } | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const serverInfoRef = useRef<ServerInfo | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const intentionalCloseRef = useRef(false);

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() => {
      if (scrollRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      }
    });
  }, []);

  // --- Bootstrap: ensure server is running, then connect WS ---
  useEffect(() => {
    let cancelled = false;

    const bootstrap = async () => {
      try {
        // Fetch LLM info for display
        const cfg = await GetLLMConfig();
        if (!cancelled) setLlm(cfg as LLMInfo);
      } catch (e) {
        // non-fatal
      }

      // Start server if not running
      try {
        const running = await IsRunning();
        if (!running) {
          setStatus("Starting local server...");
          await StartServer();
        }
      } catch (e: any) {
        if (!cancelled) setStatus(`Failed to start server: ${e?.message || e}`);
        return;
      }

      // Wait a tick for the HTTP listener to be ready, then fetch info
      await new Promise((r) => setTimeout(r, 400));

      let info: ServerInfo;
      try {
        info = (await GetServerInfo()) as ServerInfo;
      } catch (e: any) {
        if (!cancelled) setStatus(`Failed to get server info: ${e?.message || e}`);
        return;
      }

      if (cancelled) return;

      if (!info.admin_token) {
        setStatus("Server started but no admin token available.");
        return;
      }

      serverInfoRef.current = info;
      connectWS(info);
    };

    bootstrap();

    return () => {
      cancelled = true;
      intentionalCloseRef.current = true;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const scheduleReconnect = useCallback(() => {
    if (intentionalCloseRef.current) return;
    if (reconnectTimerRef.current) return;
    const info = serverInfoRef.current;
    if (!info) return;

    reconnectTimerRef.current = setTimeout(async () => {
      reconnectTimerRef.current = null;
      try {
        const running = await IsRunning();
        if (!running) {
          setStatus("Restarting server...");
          await StartServer();
          // Refresh info in case port/token changed
          const fresh = (await GetServerInfo()) as ServerInfo;
          serverInfoRef.current = fresh;
          connectWS(fresh);
          return;
        }
      } catch (e: any) {
        setStatus(`Reconnect failed: ${e?.message || e}`);
        scheduleReconnect();
        return;
      }
      connectWS(serverInfoRef.current!);
    }, 1500);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const connectWS = useCallback((info: ServerInfo) => {
    setStatus("Connecting...");
    const url = `ws://127.0.0.1:${info.port}/ws?token=${encodeURIComponent(
      info.admin_token
    )}&client_type=cli`;

    const ws = new WebSocket(url);
    wsRef.current = ws;
    intentionalCloseRef.current = false;

    ws.onopen = () => {
      setConnected(true);
      setStatus("Ready");
    };

    ws.onclose = () => {
      setConnected(false);
      setStatus("Disconnected — reconnecting...");
      if (!intentionalCloseRef.current) {
        scheduleReconnect();
      }
    };

    ws.onerror = () => {
      setStatus("Connection error");
    };

    ws.onmessage = (evt) => {
      let data: any;
      try {
        data = JSON.parse(evt.data);
      } catch {
        return;
      }

      // Debug: log every received WS message (visible in DevTools console)
      console.log("[ws]", data.type, data);

      switch (data.type) {
        case "user_echo":
          // Server confirmed the user message — nothing to do, we already show it.
          break;

        case "progress":
          setLoading(true);
          break;

        case "progress_structured": {
          setLoading(true);
          const p = data.progress || {};
          if (typeof p.thinking === "string" && p.thinking.trim()) {
            setThinking(p.thinking);
          }
          break;
        }

        case "stream_content":
          // Streaming chunk — append to the current assistant message
          if (typeof data.content === "string" && data.content.length > 0) {
            appendAssistantChunk(data.content);
          }
          break;

        case "text":
        case "card":
        case "message":
          // Final message from the agent
          if (typeof data.content === "string" && data.content.length > 0) {
            finalizeAssistant(data.content);
          }
          setLoading(false);
          setThinking("");
          break;

        default:
          break;
      }
      scrollToBottom();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scrollToBottom]);

  const appendAssistantChunk = useCallback((chunk: string) => {
    setMessages((prev) => {
      let buf = assistantBufRef.current;
      if (!buf) {
        buf = { id: genID(), content: "" };
        assistantBufRef.current = buf;
        const appended = { id: buf.id, role: "assistant" as const, content: chunk, pending: true };
        buf.content = chunk;
        return [...prev, appended];
      }
      buf.content += chunk;
      return prev.map((m) => (m.id === buf!.id ? { ...m, content: buf!.content } : m));
    });
  }, []);

  const finalizeAssistant = useCallback((content: string) => {
    const kind = isErrorLike(content) ? ("error" as const) : undefined;
    setMessages((prev) => {
      const buf = assistantBufRef.current;
      if (buf) {
        assistantBufRef.current = null;
        return prev.map((m) =>
          m.id === buf.id ? { ...m, content, pending: false, kind } : m
        );
      }
      return [
        ...prev,
        { id: genID(), role: "assistant", content, pending: false, kind },
      ];
    });
  }, []);

  const clearContext = useCallback(() => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
    wsRef.current.send(JSON.stringify({ type: "message", content: "/new" }));
    setMessages([]);
    setLoading(false);
    setThinking("");
    assistantBufRef.current = null;
  }, []);

  const handleSend = useCallback(() => {
    const text = input.trim();
    if (!text || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;

    if (text === "/clear" || text === "/new") {
      setInput("");
      clearContext();
      return;
    }

    const msg: ChatMessage = {
      id: genID(),
      role: "user",
      content: text,
    };
    setMessages((prev) => [...prev, msg]);
    setInput("");
    setLoading(true);
    setThinking("");
    assistantBufRef.current = null;

    wsRef.current.send(JSON.stringify({ type: "message", content: text }));
    scrollToBottom();
  }, [input, scrollToBottom, clearContext]);

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className="app-shell flex h-screen flex-col">
      <div className="app-shell__glow app-shell__glow--one" />
      <div className="app-shell__glow app-shell__glow--two" />

      {/* Header */}
      <header className="relative z-10 flex items-center justify-between border-b border-white/8 bg-slate-950/40 px-6 py-3 backdrop-blur-xl">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-[linear-gradient(135deg,rgba(94,215,193,0.24),rgba(99,199,255,0.2))] text-white shadow-[0_10px_24px_rgba(33,154,138,0.22)]">
            <svg viewBox="0 0 24 24" fill="none" className="h-5 w-5">
              <path
                d="M7 17.5 12 6.5l5 11M9.1 13h5.8"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </div>
          <div>
            <div className="text-sm font-semibold tracking-[-0.02em] text-white">
              xbot
            </div>
            <div className="text-[11px] text-slate-400">
              {llm ? `${llm.model} · ${new URL(llm.base_url).host}` : "…"}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span
            className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] ${
              connected
                ? "border-emerald-400/20 bg-emerald-500/10 text-emerald-100"
                : "border-amber-400/20 bg-amber-500/10 text-amber-100"
            }`}
          >
            <span
              className={`h-1.5 w-1.5 rounded-full ${
                connected ? "bg-emerald-400" : "bg-amber-400"
              }`}
            />
            {connected ? "Online" : status}
          </span>
          <button
            onClick={onOpenProfile}
            title="Profile & Settings"
            className="flex h-8 w-8 items-center justify-center rounded-full border border-white/10 bg-white/[0.06] text-slate-300 transition hover:bg-white/[0.12] hover:text-white"
          >
            <svg viewBox="0 0 20 20" fill="none" className="h-4 w-4">
              <path d="M10 10.25a2.75 2.75 0 1 0 0-5.5 2.75 2.75 0 0 0 0 5.5Zm-5 5.5c0-2.42 2.24-4 5-4s5 1.58 5 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
          </button>
        </div>
      </header>

      {/* Messages */}
      <div
        ref={scrollRef}
        className="relative z-10 flex-1 overflow-auto px-6 py-6"
      >
        <div className="mx-auto max-w-3xl space-y-4">
          {messages.length === 0 && (
            <div className="surface-panel rounded-[1.75rem] px-6 py-8 text-center">
              <div className="eyebrow">Start Chatting</div>
              <h2 className="mt-3 text-xl font-semibold tracking-[-0.03em] text-white">
                Ask me anything
              </h2>
              <p className="page-copy mt-3 text-sm">
                The local agent is ready. Your messages go through the embedded
                xbot server using the auto-configured model.
              </p>
            </div>
          )}

          {messages.map((m) => (
            <MessageBubble key={m.id} message={m} />
          ))}

          {loading && (
            <div className="flex items-center gap-2 text-xs text-slate-400">
              <span className="flex gap-1">
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-cyan-300" />
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-cyan-300 [animation-delay:120ms]" />
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-cyan-300 [animation-delay:240ms]" />
              </span>
              <span className="truncate">
                {thinking ? `Thinking: ${thinking}` : "Thinking..."}
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Input */}
      <div className="relative z-10 border-t border-white/8 bg-slate-950/40 px-6 py-4 backdrop-blur-xl">
        <div className="mx-auto flex max-w-3xl items-end gap-2">
          <button
            onClick={clearContext}
            disabled={!connected || messages.length === 0}
            title="Clear context"
            className="flex h-[44px] w-[44px] shrink-0 items-center justify-center rounded-2xl border border-white/10 bg-white/[0.04] text-slate-400 transition hover:bg-white/[0.08] hover:text-white disabled:opacity-30"
          >
            <svg viewBox="0 0 20 20" fill="none" className="h-4 w-4">
              <path d="M4 4l12 12M16 4L4 16" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
          </button>
          <textarea
            value={input}
            onChange={(e) => setInput(e.currentTarget.value)}
            onKeyDown={onKeyDown}
            placeholder={
              connected
                ? "Message xbot... (Enter to send, Shift+Enter for newline)"
                : "Waiting for connection..."
            }
            rows={1}
            disabled={!connected}
            className="max-h-40 min-h-[44px] flex-1 resize-none rounded-2xl border border-white/10 bg-slate-900/60 px-4 py-3 text-sm text-white placeholder:text-slate-500 focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10 disabled:opacity-50"
          />
          <button
            onClick={handleSend}
            disabled={!connected || !input.trim()}
            className="rounded-2xl border border-cyan-300/20 bg-cyan-500/14 px-4 py-3 text-sm font-medium text-cyan-50 transition hover:bg-cyan-500/22 disabled:cursor-not-allowed disabled:opacity-40"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  );
}

function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === "user";
  const isError = message.kind === "error";

  let bubbleClass: string;
  if (isUser) {
    bubbleClass = "border border-cyan-300/20 bg-cyan-500/12 text-cyan-50";
  } else if (isError) {
    bubbleClass = "border border-rose-400/30 bg-rose-500/10 text-rose-100";
  } else {
    bubbleClass = "surface-panel text-slate-100";
  }

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[85%] rounded-[1.25rem] px-4 py-3 text-sm leading-6 ${bubbleClass}`}
      >
        {isError && (
          <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-rose-200">
            <svg viewBox="0 0 20 20" fill="none" className="h-3.5 w-3.5">
              <circle cx="10" cy="10" r="8" stroke="currentColor" strokeWidth="1.5" />
              <path d="M10 6v4M10 13.5h.01" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            </svg>
            Error
          </div>
        )}
        <pre className="whitespace-pre-wrap break-words font-sans">
          {message.content || (message.pending ? "…" : "")}
        </pre>
      </div>
    </div>
  );
}
