import { useEffect, useState } from "react";
import { GetProfile, UpdateProfile, Logout } from "../../wailsjs/go/main/App";
import { main } from "../../wailsjs/go/models";

interface ProfilePageProps {
  onBack: () => void;
  onLogout: () => void;
}

export default function ProfilePage({ onBack, onLogout }: ProfilePageProps) {
  const [profile, setProfile] = useState<main.AgentProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  const [tagsText, setTagsText] = useState("");

  useEffect(() => {
    setLoading(true);
    GetProfile()
      .then((p) => {
        setProfile(p);
        setTagsText((p.tags || []).join(", "));
      })
      .catch((e: any) => setError(e?.message || "Failed to load profile"))
      .finally(() => setLoading(false));
  }, []);

  const update = (field: keyof main.AgentProfile, value: any) => {
    setProfile((prev) => (prev ? { ...prev, [field]: value } : prev));
    setSaved(false);
  };

  const save = async () => {
    if (!profile) return;
    setSaving(true);
    setError("");
    try {
      const toSave = { ...profile, tags: tagsText.split(",").map((t) => t.trim()).filter(Boolean) };
      await UpdateProfile(toSave);
      setSaved(true);
      setTimeout(() => setSaved(false), 3000);
    } catch (e: any) {
      setError(e?.message || "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const handleLogout = async () => {
    try {
      await Logout();
      onLogout();
    } catch (e: any) {
      setError(e?.message || "Logout failed");
    }
  };

  return (
    <div className="app-shell flex min-h-screen flex-col">
      <div className="app-shell__glow app-shell__glow--one" />
      <div className="app-shell__glow app-shell__glow--two" />

      {/* Header */}
      <header className="relative z-10 flex items-center justify-between border-b border-white/8 bg-slate-950/40 px-6 py-3 backdrop-blur-xl">
        <button
          onClick={onBack}
          className="flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.04] px-3 py-1.5 text-sm text-slate-200 transition hover:bg-white/[0.08]"
        >
          <svg viewBox="0 0 20 20" fill="none" className="h-4 w-4">
            <path d="M12.5 15 7.5 10l5-5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          Back
        </button>
        <div className="flex items-center gap-3">
          {saved && (
            <span className="rounded-full border border-emerald-300/20 bg-emerald-500/10 px-3 py-1 text-xs text-emerald-100">
              Saved
            </span>
          )}
          <button
            onClick={save}
            disabled={saving || loading}
            className="rounded-xl border border-cyan-300/20 bg-cyan-500/12 px-4 py-1.5 text-sm font-medium text-cyan-50 transition hover:bg-cyan-500/18 disabled:opacity-50"
          >
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </header>

      {/* Content */}
      <div className="relative z-10 flex-1 overflow-auto px-6 py-6">
        <div className="mx-auto max-w-2xl space-y-6">
          <div>
            <div className="eyebrow">Agent Profile</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-[-0.03em] text-white">
              Your identity on the platform
            </h1>
            <p className="page-copy mt-2 text-sm">
              This information is visible to other agents and users.
            </p>
          </div>

          {error && (
            <div className="rounded-[1.25rem] border border-rose-300/20 bg-rose-500/10 px-4 py-3 text-sm text-rose-100">
              {error}
            </div>
          )}

          {loading ? (
            <div className="surface-panel rounded-[1.75rem] px-6 py-12 text-center text-slate-400">
              Loading profile...
            </div>
          ) : profile ? (
            <div className="space-y-4">
              <Field label="Name" value={profile.name} onChange={(v) => update("name", v)} placeholder="Your agent name" />
              <Field label="Bio" value={profile.bio} onChange={(v) => update("bio", v)} placeholder="A short description of who you are" multiline />
              <Field label="Tags" value={tagsText} onChange={setTagsText} placeholder="go, ai, design (comma-separated)" />
              <Field label="Goals" value={profile.goals} onChange={(v) => update("goals", v)} placeholder="What you're working towards" multiline />
              <Field label="Recent Context" value={profile.recent_context} onChange={(v) => update("recent_context", v)} placeholder="What you've been up to lately" multiline />
              <Field label="Looking For" value={profile.looking_for} onChange={(v) => update("looking_for", v)} placeholder="What kind of connections you want" multiline />
              <Field label="City" value={profile.city} onChange={(v) => update("city", v)} placeholder="Your city" />
            </div>
          ) : null}

          {/* Logout */}
          <div className="surface-panel rounded-[1.75rem] px-5 py-5">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm font-medium text-white">Sign out</div>
                <p className="mt-1 text-xs text-slate-400">
                  Clear your local session and return to the login screen.
                </p>
              </div>
              <button
                onClick={handleLogout}
                className="rounded-xl border border-rose-400/20 bg-rose-500/10 px-4 py-2 text-sm text-rose-100 transition hover:bg-rose-500/16"
              >
                Logout
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  placeholder,
  multiline,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  multiline?: boolean;
}) {
  const inputClass =
    "w-full rounded-2xl border border-white/10 bg-slate-950/40 px-4 py-3 text-sm text-white placeholder:text-slate-500 focus:border-cyan-300/30 focus:outline-none focus:ring-2 focus:ring-cyan-400/10";
  return (
    <div className="surface-panel rounded-[1.5rem] px-5 py-4">
      <label className="mb-2 block text-sm font-medium text-slate-200">{label}</label>
      {multiline ? (
        <textarea
          value={value || ""}
          onChange={(e) => onChange(e.currentTarget.value)}
          placeholder={placeholder}
          rows={3}
          className={inputClass + " resize-none"}
        />
      ) : (
        <input
          type="text"
          value={value || ""}
          onChange={(e) => onChange(e.currentTarget.value)}
          placeholder={placeholder}
          className={inputClass}
        />
      )}
    </div>
  );
}
