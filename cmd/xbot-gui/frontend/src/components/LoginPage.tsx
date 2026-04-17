import { useState } from "react";
import { countryCodeOptions, normalizePhoneNumber } from "../lib/phoneAuth";

interface LoginPageProps {
  onLogin: (
    countryCode: string,
    phoneNumber: string,
    verificationCode: string
  ) => Promise<void>;
  onSendCode: (
    countryCode: string,
    phoneNumber: string
  ) => Promise<{ deliveryMessage: string; expiresInSeconds: number }>;
}

export default function LoginPage({ onLogin, onSendCode }: LoginPageProps) {
  const [countryCode, setCountryCode] = useState("+86");
  const [phoneNumber, setPhoneNumber] = useState("");
  const [verificationCode, setVerificationCode] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [sendingCode, setSendingCode] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const handleSendCode = async () => {
    const normalizedPhone = normalizePhoneNumber(phoneNumber);
    if (!normalizedPhone) {
      setError("Enter your phone number before requesting a verification code.");
      return;
    }

    setSendingCode(true);
    setError("");
    try {
      const result = await onSendCode(countryCode, normalizedPhone);
      setPhoneNumber(normalizedPhone);
      setMessage(
        result.deliveryMessage ||
          `Verification code sent. Expires in ${result.expiresInSeconds} seconds.`
      );
    } catch (e: any) {
      setError(e?.message || "Failed to send verification code");
    } finally {
      setSendingCode(false);
    }
  };

  const handleSubmit = async () => {
    const normalizedPhone = normalizePhoneNumber(phoneNumber);
    if (!normalizedPhone) {
      setError("Enter the phone number used for desktop login.");
      return;
    }
    if (!verificationCode.trim()) {
      setError("Enter the verification code to continue.");
      return;
    }

    setSubmitting(true);
    setError("");
    try {
      await onLogin(countryCode, normalizedPhone, verificationCode.trim());
      setVerificationCode("");
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
      <div className="surface-panel panel-grid relative z-10 w-full max-w-5xl overflow-hidden rounded-[2.25rem]">
        <div className="grid gap-0 lg:grid-cols-[1.15fr_0.85fr]">
          <section className="border-b border-white/8 px-6 py-8 lg:border-b-0 lg:border-r lg:px-8 lg:py-10">
            <div className="eyebrow">Desktop Access</div>
            <h1 className="mt-3 page-title max-w-xl">
              Unlock the xbot control room.
            </h1>
            <p className="page-copy mt-4 max-w-2xl text-sm md:text-base">
              Sign in with the trusted phone number and a one-time verification
              code. The country code selector is built into the phone field, so
              switching regions stays quick.
            </p>

            <div className="mt-8 grid gap-3 sm:grid-cols-3">
              {[
                "Enter the phone number bound to your xbot account.",
                "Verification codes expire after 5 minutes; request a new one if it times out.",
                "Sessions persist locally until you explicitly log out from the sidebar.",
              ].map((note, index) => (
                <div
                  key={note}
                  className="surface-outline rounded-[1.5rem] px-4 py-4"
                >
                  <div className="text-[11px] uppercase tracking-[0.2em] text-slate-500">
                    Guard {index + 1}
                  </div>
                  <p className="mt-3 text-sm leading-6 text-slate-200">{note}</p>
                </div>
              ))}
            </div>
          </section>

          <section className="px-6 py-8 lg:px-8 lg:py-10">
            <div className="surface-outline rounded-[1.75rem] px-5 py-5">
              <div className="text-[11px] uppercase tracking-[0.22em] text-slate-500">
                Sign In
              </div>
              <h2 className="mt-3 text-2xl font-semibold tracking-[-0.04em] text-white">
                Phone verification login
              </h2>
              <p className="page-copy mt-3 text-sm">
                Enter your phone number, request a code, then paste the
                verification code to unlock the desktop GUI.
              </p>

              <div className="mt-6 space-y-2">
                <label className="block text-sm font-medium text-slate-200">
                  Phone Number
                </label>
                <div className="flex gap-3">
                  <select
                    value={countryCode}
                    onChange={(e) => setCountryCode(e.currentTarget.value)}
                    className="w-32 rounded-2xl border border-white/10 bg-slate-950/40 px-4 py-3 text-sm text-white focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10"
                  >
                    {countryCodeOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                  <input
                    type="tel"
                    value={phoneNumber}
                    autoFocus
                    onChange={(e) => setPhoneNumber(normalizePhoneNumber(e.currentTarget.value))}
                    placeholder="13800138000"
                    className="flex-1 rounded-2xl border border-white/10 bg-slate-950/40 px-4 py-3 text-sm text-white placeholder:text-slate-500 focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10"
                  />
                </div>
              </div>

              <div className="mt-4 space-y-2">
                <label className="block text-sm font-medium text-slate-200">
                  Verification Code
                </label>
                <div className="flex gap-3">
                  <input
                    type="text"
                    value={verificationCode}
                    onChange={(e) => setVerificationCode(e.currentTarget.value.replace(/\D+/g, "").slice(0, 6))}
                    onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
                    placeholder="6-digit code"
                    className="flex-1 rounded-2xl border border-white/10 bg-slate-950/40 px-4 py-3 text-sm text-white placeholder:text-slate-500 focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10"
                  />
                  <button
                    onClick={handleSendCode}
                    disabled={sendingCode}
                    className="rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3 text-sm font-medium text-slate-200 transition hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {sendingCode ? "Sending..." : "Send Code"}
                  </button>
                </div>
              </div>

              {message && (
                <div className="mt-4 rounded-[1.25rem] border border-cyan-300/20 bg-cyan-500/10 px-4 py-3 text-sm text-cyan-100">
                  {message}
                </div>
              )}

              {error && (
                <div className="mt-4 rounded-[1.25rem] border border-rose-300/20 bg-rose-500/10 px-4 py-3 text-sm text-rose-100">
                  {error}
                </div>
              )}

              <button
                onClick={handleSubmit}
                disabled={submitting}
                className="mt-6 w-full rounded-2xl border border-cyan-300/20 bg-cyan-500/12 px-5 py-3 text-sm font-medium text-cyan-50 transition hover:bg-cyan-500/18 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {submitting ? "Verifying..." : "Sign In"}
              </button>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
