import { useEffect, useState } from "react";
import {
  GetAuthStatus,
  Logout as LogoutApp,
  RequestDesktopLoginCode,
  VerifyDesktopLoginCode,
} from "../wailsjs/go/main/App";
import ChatPage from "./components/ChatPage";
import LoginPage from "./components/LoginPage";

interface AuthStatus {
  authenticated: boolean;
}

export default function App() {
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [authError, setAuthError] = useState("");

  const refreshAuth = async () => {
    try {
      const status = await GetAuthStatus();
      setAuthStatus({ authenticated: Boolean(status?.authenticated) });
      setAuthError("");
    } catch (e: any) {
      setAuthError(e?.message || "Failed to check login status");
    }
  };

  useEffect(() => {
    refreshAuth();
  }, []);

  const handleLogin = async (
    countryCode: string,
    phoneNumber: string,
    verificationCode: string
  ) => {
    await VerifyDesktopLoginCode(countryCode, phoneNumber, verificationCode);
    await refreshAuth();
  };

  const handleSendCode = async (countryCode: string, phoneNumber: string) => {
    const result = await RequestDesktopLoginCode(countryCode, phoneNumber);
    return {
      deliveryMessage: String(result?.delivery_message || ""),
      expiresInSeconds: Number(result?.expires_in_seconds || 0),
    };
  };

  const handleLogout = async () => {
    try {
      await LogoutApp();
      setAuthStatus({ authenticated: false });
    } catch (e: any) {
      alert("Failed to log out: " + (e?.message || e));
    }
  };

  if (!authStatus && !authError) {
    return (
      <div className="app-shell flex min-h-screen items-center justify-center px-6">
        <div className="surface-panel panel-grid w-full max-w-xl rounded-[2rem] px-6 py-8 text-center">
          <div className="eyebrow">Session Check</div>
          <h1 className="mt-3 page-title">Opening the workspace</h1>
          <p className="page-copy mt-4 text-sm md:text-base">
            Checking whether you are already signed in.
          </p>
        </div>
      </div>
    );
  }

  if (authError) {
    return (
      <div className="app-shell flex min-h-screen items-center justify-center px-6">
        <div className="surface-panel panel-grid w-full max-w-xl rounded-[2rem] px-6 py-8 text-center">
          <div className="eyebrow">Authentication Error</div>
          <h1 className="mt-3 page-title">Unable to verify login state</h1>
          <p className="page-copy mt-4 text-sm md:text-base">{authError}</p>
          <button
            onClick={refreshAuth}
            className="mx-auto mt-6 rounded-2xl border border-cyan-300/20 bg-cyan-500/12 px-5 py-3 text-sm font-medium text-cyan-50 transition hover:bg-cyan-500/18"
          >
            Try Again
          </button>
        </div>
      </div>
    );
  }

  if (!authStatus?.authenticated) {
    return <LoginPage onLogin={handleLogin} onSendCode={handleSendCode} />;
  }

  return <ChatPage onLogout={handleLogout} />;
}
