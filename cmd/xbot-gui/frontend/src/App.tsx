import { useEffect, useState } from "react";
import {
  GetAuthStatus,
  Login,
  RefreshToken,
  SendLoginCode,
} from "../wailsjs/go/main/App";
import ChatPage from "./components/ChatPage";
import LoginPage from "./components/LoginPage";
import ProfilePage from "./components/ProfilePage";

type Page = "login" | "chat" | "profile";

export default function App() {
  const [page, setPage] = useState<Page | null>(null);
  const [authError, setAuthError] = useState("");

  // Check auth on startup: verify local session then validate with server
  useEffect(() => {
    (async () => {
      try {
        const status = await GetAuthStatus();
        if (!status?.authenticated) {
          setPage("login");
          return;
        }
        // Token exists locally — verify it's still valid
        const refreshResult = await RefreshToken();
        if (refreshResult?.authenticated) {
          setPage("chat");
        } else {
          setPage("login");
        }
        setAuthError("");
      } catch (e: any) {
        setAuthError(e?.message || "Failed to check auth");
        setPage("login");
      }
    })();
  }, []);

  // Periodic JWT refresh (every 10 minutes)
  useEffect(() => {
    if (page !== "chat" && page !== "profile") return;

    const interval = setInterval(async () => {
      try {
        const result = await RefreshToken();
        if (!result?.authenticated) {
          setPage("login");
        }
      } catch {
        // refresh failed silently, next interval will retry
      }
    }, 10 * 60 * 1000);

    return () => clearInterval(interval);
  }, [page]);

  const handleLogin = async (
    countryCode: string,
    phoneNumber: string,
    code: string
  ) => {
    await Login(countryCode, phoneNumber, code);
    setPage("chat");
  };

  const handleSendCode = async (countryCode: string, phoneNumber: string) => {
    const result = await SendLoginCode(countryCode, phoneNumber);
    return { message: String(result?.message || "Code sent.") };
  };

  const handleLogout = () => {
    setPage("login");
  };

  // Loading state
  if (page === null) {
    return (
      <div className="app-shell flex min-h-screen items-center justify-center px-6">
        <div className="surface-panel panel-grid w-full max-w-sm rounded-[2rem] px-6 py-8 text-center">
          <div className="eyebrow">xbot</div>
          <p className="page-copy mt-3 text-sm">
            {authError || "Checking session..."}
          </p>
        </div>
      </div>
    );
  }

  if (page === "login") {
    return <LoginPage onLogin={handleLogin} onSendCode={handleSendCode} />;
  }

  if (page === "profile") {
    return (
      <ProfilePage
        onBack={() => setPage("chat")}
        onLogout={handleLogout}
      />
    );
  }

  return <ChatPage onOpenProfile={() => setPage("profile")} />;
}
