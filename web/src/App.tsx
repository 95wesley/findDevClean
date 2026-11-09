import { useEffect, useMemo, useState } from "react";
import { api, type Batch, type CleanReport, type Finding, type Risk, type ScanResult } from "./api";

const RISK_LABEL: Record<Risk, string> = {
  safe: "seguro",
  regenerable: "regenerável",
  caution: "cuidado",
};

const STATUS_LABEL: Record<string, string> = {
  would_remove: "seria removido",
  removed: "movido para quarentena",
  restored: "restaurado",
  rejected: "recusado",
  failed: "falhou",
};

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
  const [scan, setScan] = useState<ScanResult | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [dryRun, setDryRun] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [report, setReport] = useState<CleanReport | null>(null);
  const [batches, setBatches] = useState<Batch[]>([]);

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
    return [...groups.entries()];
  }, [scan]);

  async function guard<T>(label: string, fn: () => Promise<T>): Promise<T | undefined> {
    setBusy(label);
    setError(null);
    try {
      return await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return undefined;
    } finally {
      setBusy(null);
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
    const res = await guard("scan", () => api.scan());
    if (res) {
      setScan(res);
      setSelected(new Set());
      setReport(null);
    }
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
        `Mover ${paths.length} item(ns) (${formatBytes(selectedSize)}) para a quarentena?\n` +
          "Você pode desfazer depois na lista de lotes.",
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
      setError(
        failed > 0
          ? `${restored} restaurado(s); ${failed} não puderam ser restaurados (destino ocupado ou erro).`
          : null,
      );
    }
  }

  return (
    <main className="wrap">
      <header className="top">
        <div>
          <h1>findDevClean</h1>
          <p className="muted">Encontre e libere espaço de caches e artefatos de build.</p>
        </div>
        <button className="primary" onClick={runScan} disabled={busy !== null}>
          {busy === "scan" ? "Escaneando…" : "Escanear"}
        </button>
      </header>

      {error && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}

      {scan && (
        <section aria-label="Resultados">
          <div className="summary">
            <strong>{formatBytes(scan.total)}</strong>
            <span className="muted">
              {" "}
              recuperáveis em {scan.findings.length} itens
              {scan.errors.length > 0 && ` · ${scan.errors.length} caminhos ignorados`}
            </span>
          </div>

          {scan.findings.length === 0 ? (
            <p className="muted">Nada encontrado. Seu ambiente está limpo.</p>
          ) : (
            <>
              <div className="toolbar">
                <button onClick={selectSafe} disabled={busy !== null}>
                  Selecionar seguros
                </button>
                <button onClick={() => setSelected(new Set())} disabled={selected.size === 0}>
                  Limpar seleção
                </button>
              </div>

              {grouped.map(([category, items]) => (
                <div className="group" key={category}>
                  <h2>{category}</h2>
                  <ul className="list">
                    {items.map((f) => (
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
                              <span className={`badge ${f.risk}`}>{RISK_LABEL[f.risk]}</span>
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
                <div>
                  <strong>{formatBytes(selectedSize)}</strong>
                  <span className="muted"> selecionados ({selected.size})</span>
                </div>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={dryRun}
                    onChange={(e) => setDryRun(e.target.checked)}
                  />
                  Simular (não move nada)
                </label>
                <button
                  className="primary"
                  onClick={runClean}
                  disabled={busy !== null || selected.size === 0}
                >
                  {busy === "clean" ? "Processando…" : dryRun ? "Simular limpeza" : "Limpar"}
                </button>
              </div>
            </>
          )}
        </section>
      )}

      {report && (
        <section className="report" aria-label="Resultado da limpeza">
          <h2>{report.dry_run ? "Simulação" : "Limpeza concluída"}</h2>
          <p className="muted">
            {report.dry_run ? "Liberaria" : "Liberou"} {formatBytes(report.freed)}
            {report.batch_id && " · lote criado, você pode desfazer abaixo"}
          </p>
          <ul className="list">
            {report.outcomes.map((o) => (
              <li key={o.path} className="item small">
                <span className={`status ${o.status}`}>{STATUS_LABEL[o.status] ?? o.status}</span>{" "}
                <code className="path">{o.path}</code>
                {o.reason && <span className="muted"> — {o.reason}</span>}
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-label="Lotes">
        <h2>Quarentena</h2>
        {batches.length === 0 ? (
          <p className="muted">Nenhum lote. Itens movidos aparecem aqui para desfazer.</p>
        ) : (
          <ul className="list">
            {batches.map((b) => (
              <li key={b.id} className="item">
                <div className="grow">
                  <strong>{new Date(b.created_at).toLocaleString("pt-BR")}</strong>
                  <span className="muted small">
                    {" "}
                    · {b.entries.length} item(ns) ·{" "}
                    {formatBytes(b.entries.reduce((s, e) => s + e.size, 0))}
                  </span>
                </div>
                <button onClick={() => undo(b.id)} disabled={busy !== null}>
                  Desfazer
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
