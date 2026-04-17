import { useState } from "react";
import { countryCodeOptions, normalizePhoneNumber } from "../lib/phoneAuth";

interface LoginPageProps {
  onLogin: (countryCode: string, phoneNumber: string, code: string) => Promise<void>;
  onSendCode: (countryCode: string, phoneNumber: string) => Promise<{ message: string }>;
}

export default function LoginPage({ onLogin, onSendCode }: LoginPageProps) {
  const [countryCode, setCountryCode] = useState("+86");
  const [phoneNumber, setPhoneNumber] = useState("");
  const [code, setCode] = useState("");
  const [codeSent, setCodeSent] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [sendingCode, setSendingCode] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const handleSendCode = async () => {
    const phone = normalizePhoneNumber(phoneNumber);
    if (!phone) {
      setError("Please enter your phone number.");
      return;
    }
    setSendingCode(true);
    setError("");
    try {
      const result = await onSendCode(countryCode, phone);
      setPhoneNumber(phone);
      setCodeSent(true);
      setMessage(result.message || "Verification code sent.");
    } catch (e: any) {
      setError(e?.message || "Failed to send verification code");
    } finally {
      setSendingCode(false);
    }
  };

  const handleSubmit = async () => {
    const phone = normalizePhoneNumber(phoneNumber);
    if (!phone) { setError("Please enter your phone number."); return; }
    if (!code.trim()) { setError("Please enter the verification code."); return; }
    setSubmitting(true);
    setError("");
    try {
      await onLogin(countryCode, phone, code.trim());
    } catch (e: any) {
      setError(e?.message || "Login failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="app-shell flex min-h-screen items-center justify-center px-6 py-10">
      <div className="app-shell__glow app-shell__glow--one" />
      <div className="app-shell__glow app-shell__glow--two" />
      <div className="surface-panel panel-grid relative z-10 w-full max-w-md overflow-hidden rounded-[2.25rem] px-6 py-8">
        <div className="text-center">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-[linear-gradient(135deg,rgba(94,215,193,0.24),rgba(99,199,255,0.2))] text-white shadow-[0_12px_30px_rgba(33,154,138,0.18)]">
            <svg viewBox="0 0 24 24" fill="none" className="h-7 w-7">
              <path d="M7 17.5 12 6.5l5 11M9.1 13h5.8" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </div>
          <h1 className="mt-4 text-2xl font-semibold tracking-[-0.03em] text-white">
            Sign in to xbot
          </h1>
          <p className="page-copy mt-2 text-sm">
            Enter your phone number to get started.
          </p>
        </div>

        <div className="mt-8 space-y-4">
          <div className="space-y-2">
            <label className="block text-sm font-medium text-slate-200">Phone Number</label>
            <div className="flex gap-2">
              <select
                value={countryCode}
                onChange={(e) => setCountryCode(e.currentTarget.value)}
                className="w-28 rounded-2xl border border-white/10 bg-slate-950/40 px-3 py-3 text-sm text-white focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10"
              >
                {countryCodeOptions.map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
              <input
                type="tel"
                value={phoneNumber}
                autoFocus
                onChange={(e) => setPhoneNumber(e.currentTarget.value)}
                placeholder="13800138000"
                className="flex-1 rounded-2xl border border-white/10 bg-slate-950/40 px-4 py-3 text-sm text-white placeholder:text-slate-500 focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10"
              />
            </div>
          </div>

          {codeSent && (
            <div className="space-y-2">
              <label className="block text-sm font-medium text-slate-200">Verification Code</label>
              <input
                type="text"
                value={code}
                autoFocus
                onChange={(e) => setCode(e.currentTarget.value.replace(/\D+/g, "").slice(0, 6))}
                onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
                placeholder="6-digit code"
                className="w-full rounded-2xl border border-white/10 bg-slate-950/40 px-4 py-3 text-sm text-white placeholder:text-slate-500 focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10"
              />
            </div>
          )}

          {message && (
            <div className="rounded-[1.25rem] border border-cyan-300/20 bg-cyan-500/10 px-4 py-3 text-sm text-cyan-100">
              {message}
            </div>
          )}
          {error && (
            <div className="rounded-[1.25rem] border border-rose-300/20 bg-rose-500/10 px-4 py-3 text-sm text-rose-100">
              {error}
            </div>
          )}

          {!codeSent ? (
            <button
              onClick={handleSendCode}
              disabled={sendingCode}
              className="w-full rounded-2xl border border-cyan-300/20 bg-cyan-500/12 px-5 py-3 text-sm font-medium text-cyan-50 transition hover:bg-cyan-500/18 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {sendingCode ? "Sending..." : "Send Verification Code"}
            </button>
          ) : (
            <div className="flex gap-2">
              <button
                onClick={handleSubmit}
                disabled={submitting}
                className="flex-1 rounded-2xl border border-cyan-300/20 bg-cyan-500/12 px-5 py-3 text-sm font-medium text-cyan-50 transition hover:bg-cyan-500/18 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {submitting ? "Signing in..." : "Sign In"}
              </button>
              <button
                onClick={handleSendCode}
                disabled={sendingCode}
                className="rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3 text-sm text-slate-300 transition hover:bg-white/[0.08] disabled:opacity-50"
              >
                {sendingCode ? "..." : "Resend"}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
