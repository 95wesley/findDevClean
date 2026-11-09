import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import Permissions from "./Permissions";
import DiskMap from "./DiskMap";
import {
  api,
  type Batch,
  type CleanReport,
  type Finding,
  type ScanProgress,
  type ScanResult,
} from "./api";
import { LANGS, type Key, type Lang, loadLang, makeT } from "./i18n";

const HeroShader = lazy(() => import("./HeroShader"));

export function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let v = b / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

export default function App() {
  const [lang, setLang] = useState<Lang>(loadLang);
  const t = useMemo(() => makeT(lang), [lang]);

  const [scan, setScan] = useState<ScanResult | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [dryRun, setDryRun] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [progress, setProgress] = useState<ScanProgress | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [report, setReport] = useState<CleanReport | null>(null);
  const [batches, setBatches] = useState<Batch[]>([]);
  const [showPerms, setShowPerms] = useState(false);

  // The background effect is decorative: it is dropped when WebGPU is missing,
  // when the user asks for less motion, or when the user turns it off.
  const [gpuOk, setGpuOk] = useState(true);
  const reduceMotion = useMemo(
    () => window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false,
    [],
  );
  const [fxOn, setFxOn] = useState<boolean>(() => {
    try {
      return localStorage.getItem("findclean-fx") !== "off";
    } catch {
      return true;
    }
  });

  useEffect(() => {
    document.documentElement.lang = lang;
    try {
      localStorage.setItem("findclean-lang", lang);
    } catch {
      // Storage blocked: the choice only lasts for this page view.
    }
  }, [lang]);

  useEffect(() => {
    try {
      localStorage.setItem("findclean-fx", fxOn ? "on" : "off");
    } catch {
      // Storage blocked: the choice only lasts for this page view.
    }
  }, [fxOn]);

  const selectedSize = useMemo(
    () =>
      (scan?.findings ?? [])
        .filter((f) => selected.has(f.path))
        .reduce((sum, f) => sum + f.size, 0),
    [scan, selected],
  );

  const grouped = useMemo(() => {
    const groups = new Map<string, Finding[]>();
    for (const f of scan?.findings ?? []) {
      const list = groups.get(f.category) ?? [];
      list.push(f);
      groups.set(f.category, list);
    }
    return [...groups.entries()].map(([category, items]) => ({
      category,
      items,
      size: items.reduce((s, f) => s + f.size, 0),
    }));
  }, [scan]);

  async function guard<T>(label: string, fn: () => Promise<T>): Promise<T | undefined> {
    // The quarantine refresh runs in the background and must not disable actions.
    const blocking = label !== "batches";
    if (blocking) setBusy(label);
    setError(null);
    try {
      return await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return undefined;
    } finally {
      if (blocking) setBusy(null);
    }
  }

  async function refreshBatches() {
    const b = await guard("batches", () => api.batches());
    if (b) setBatches(b);
  }

  useEffect(() => {
    void refreshBatches();
    // Initial load only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function runScan() {
    setProgress({ phase: "walk", done: 0, total: 0 });
    const res = await guard("scan", () => api.scan(setProgress));
    setProgress(null);
    if (res) {
      setScan(res);
      setSelected(new Set());
      setReport(null);
    }
  }

  // Share of the scan done. The walk is 85% of the bar and the caches 15%.
  function percentOf(p: ScanProgress | null): number {
    if (!p) return 0;
    if (p.phase === "done") return 100;
    const ratio = p.total > 0 ? Math.min(1, p.done / p.total) : 0;
    return p.phase === "walk" ? Math.round(ratio * 85) : 85 + Math.round(ratio * 15);
  }

  function toggle(path: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }

  function selectSafe() {
    setSelected(
      new Set((scan?.findings ?? []).filter((f) => f.risk === "safe").map((f) => f.path)),
    );
  }

  async function runClean() {
    const paths = [...selected];
    if (paths.length === 0) return;
    if (!dryRun) {
      const ok = window.confirm(
        t("confirmClean", { n: paths.length, size: formatBytes(selectedSize) }),
      );
      if (!ok) return;
    }
    const res = await guard("clean", () => api.clean(paths, dryRun));
    if (res) {
      setReport(res);
      if (!res.dry_run) {
        setScan(null);
        setSelected(new Set());
        void refreshBatches();
      }
    }
  }

  async function undo(batchId: string) {
    const res = await guard("undo", () => api.undo(batchId));
    if (res) {
      setReport(null);
      void refreshBatches();
      const restored = res.outcomes.filter((o) => o.status === "restored").length;
      const failed = res.outcomes.length - restored;
      if (failed > 0) setError(t("undoPartial", { ok: restored, fail: failed }));
    }
  }

  const effectOn = fxOn && gpuOk && !reduceMotion;

  return (
    <main className="wrap">
      {effectOn && (
        <Suspense fallback={null}>
          <HeroShader onUnavailable={() => setGpuOk(false)} />
        </Suspense>
      )}

      <header className="top">
        <div className="brand">
          <img src="/logo.svg" alt="" aria-hidden="true" />
          <div className="brand-text">
            <h1 className="brand-name">findDevClean</h1>
          </div>
        </div>
        <div className="toolbar-right">
          <label className="visually-hidden" htmlFor="lang">
            {t("language")}
          </label>
          <select
            id="lang"
            className="lang"
            value={lang}
            onChange={(e) => setLang(e.target.value as Lang)}
          >
            {LANGS.map((l) => (
              <option key={l.code} value={l.code}>
                {l.label}
              </option>
            ))}
          </select>
          <button
            className="icon"
            onClick={() => setFxOn((v) => !v)}
            aria-pressed={effectOn}
            aria-label={effectOn ? t("fxOn") : t("fxOff")}
            title={gpuOk ? (effectOn ? t("fxOn") : t("fxOff")) : t("fxUnsupported")}
            disabled={!gpuOk}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
              <path d="M2 9c2.5-2.5 5.5-2.5 8 0s5.5 2.5 8 0M2 15c2.5-2.5 5.5-2.5 8 0s5.5 2.5 8 0" />
            </svg>
          </button>
          <button
            className="icon"
            onClick={() => setShowPerms(true)}
            aria-label={t("perms")}
            title={t("perms")}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
            </svg>
          </button>
          <button className="primary" onClick={runScan} disabled={busy !== null}>
            {busy === "scan" ? t("scanning") : t("scan")}
          </button>
        </div>
      </header>

      {error && (
        <div className="alert" role="alert">
          <span>{error}</span>
          <button className="icon ghost" onClick={() => setError(null)} aria-label={t("dismiss")}>
            ×
          </button>
        </div>
      )}

      {busy === "scan" && progress && (
        <section className="progress" aria-live="polite">
          <div className="progress-head">
            <span>
              {t("scanningPct", { pct: percentOf(progress) })}
            </span>
            <span className="muted small">
              {progress.phase === "walk" ? t("phaseWalk") : t("phaseCaches")}
            </span>
          </div>
          <div
            className="bar"
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percentOf(progress)}
          >
            <div style={{ width: `${percentOf(progress)}%` }} />
          </div>
        </section>
      )}

      {!scan && !report && busy !== "scan" && (
        <section className="hero">
          <h2>{t("subtitle")}</h2>
          <p>{t("emptyCta")}</p>
          <button className="primary" onClick={runScan} disabled={busy !== null}>
            {busy === "scan" ? t("scanning") : t("scan")}
          </button>
        </section>
      )}

      {scan && (
        <section aria-label={t("scan")}>
          <div className="stats">
            <div className="stat hot">
              <div className="stat-label">{t("statRecover")}</div>
              <div className="stat-value">{formatBytes(scan.total)}</div>
              {scan.errors.length > 0 && (
                <div className="muted small">{t("ignored", { n: scan.errors.length })}</div>
              )}
            </div>
            <div className="stat">
              <div className="stat-label">{t("statItems")}</div>
              <div className="stat-value">{scan.findings.length}</div>
            </div>
            <div className="stat">
              <div className="stat-label">{t("statSelected")}</div>
              <div className="stat-value">{formatBytes(selectedSize)}</div>
            </div>
          </div>

          {scan.findings.length === 0 ? (
            <p className="muted empty">{t("nothing")}</p>
          ) : (
            <>
              <div className="toolbar">
                <button onClick={selectSafe} disabled={busy !== null}>
                  {t("selectSafe")}
                </button>
                <button onClick={() => setSelected(new Set())} disabled={selected.size === 0}>
                  {t("clearSel")}
                </button>
              </div>

              {grouped.map((g) => (
                <div className="group" key={g.category}>
                  <div className="group-head">
                    <h2>{g.category}</h2>
                    <span className="muted small">
                      {g.items.length} · {formatBytes(g.size)}
                    </span>
                  </div>
                  <ul className="list">
                    {g.items.map((f) => (
                      <li key={f.path} className="item">
                        <label>
                          <input
                            type="checkbox"
                            checked={selected.has(f.path)}
                            onChange={() => toggle(f.path)}
                          />
                          <span className="grow">
                            <span className="row">
                              <strong>{f.name}</strong>
                              <span className={`badge ${f.risk}`}>
                                {t(`risk_${f.risk}` as Key)}
                              </span>
                            </span>
                            <code className="path">{f.path}</code>
                            <span className="muted small">{f.description}</span>
                          </span>
                          <span className="size">{formatBytes(f.size)}</span>
                        </label>
                      </li>
                    ))}
                  </ul>
                </div>
              ))}

              <div className="actionbar">
                <div className="sel">
                  <strong>{formatBytes(selectedSize)}</strong>
                  <span className="muted"> {t("selectedCount", { n: selected.size })}</span>
                </div>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={dryRun}
                    onChange={(e) => setDryRun(e.target.checked)}
                  />
                  {t("dryRun")}
                </label>
                <button
                  className="primary"
                  onClick={runClean}
                  disabled={busy !== null || selected.size === 0}
                >
                  {busy === "clean"
                    ? t("processing")
                    : dryRun
                      ? t("simulate")
                      : t("clean")}
                </button>
              </div>
            </>
          )}
        </section>
      )}

      {report && (
        <section className="report" aria-label={t("reportDone")}>
          <h2>{report.dry_run ? t("reportSim") : t("reportDone")}</h2>
          <p className="muted">
            {report.dry_run
              ? t("freedSim", { size: formatBytes(report.freed) })
              : t("freedDone", { size: formatBytes(report.freed) })}
            {report.batch_id && ` · ${t("batchNote")}`}
          </p>
          <ul className="list">
            {report.outcomes.map((o) => (
              <li key={o.path} className="item small">
                <span className={`status ${o.status}`}>{t(`st_${o.status}` as Key)}</span>{" "}
                <code className="path">{o.path}</code>
                {o.reason && <span className="muted"> — {o.reason}</span>}
              </li>
            ))}
          </ul>
        </section>
      )}

      <DiskMap t={t} fmt={formatBytes} />

      <section aria-label={t("quarantine")}>
        <h2>{t("quarantine")}</h2>
        {batches.length === 0 ? (
          <p className="muted empty">{t("noBatches")}</p>
        ) : (
          <ul className="list">
            {batches.map((b) => (
              <li key={b.id} className="item">
                <div className="grow">
                  <strong>{new Date(b.created_at).toLocaleString(lang)}</strong>
                  <span className="muted small">
                    {" "}
                    · {b.entries.length} {t("items")} ·{" "}
                    {formatBytes(b.entries.reduce((s, e) => s + e.size, 0))}
                  </span>
                </div>
                <button onClick={() => undo(b.id)} disabled={busy !== null}>
                  {t("undo")}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {showPerms && <Permissions onClose={() => setShowPerms(false)} t={t} />}
    </main>
  );
}
