import { useMemo, useState } from "react";
import { hierarchy, treemap } from "d3-hierarchy";
import { api, type TreeNode } from "./api";
import type { T } from "./i18n";

const W = 880;
const H = 420;

// Stable colour per name, so the same folder keeps its colour between views.
function hueOf(name: string): number {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return h % 360;
}

export default function DiskMap({ t, fmt }: { t: T; fmt: (bytes: number) => string }) {
  const [building, setBuilding] = useState(false);
  const [pct, setPct] = useState(0);
  const [error, setError] = useState<string | null>(null);
  // trail[0] is the home folder; the last item is the folder on screen.
  const [trail, setTrail] = useState<TreeNode[]>([]);

  const current = trail[trail.length - 1] ?? null;

  // Lay out the folder on screen: one block per child, sized by its total.
  const blocks = useMemo(() => {
    if (!current) return [];
    const root = hierarchy<TreeNode>(current, (n) => n.children)
      .sum((n) => (n.children && n.children.length > 0 ? 0 : n.size))
      .sort((a, b) => (b.value ?? 0) - (a.value ?? 0));
    const laid = treemap<TreeNode>().size([W, H]).paddingInner(3).round(true)(root);
    return (laid.children ?? []).map((c) => ({
      key: `${c.data.path ?? c.data.name}-${c.x0}-${c.y0}`,
      node: c.data,
      x: c.x0,
      y: c.y0,
      w: c.x1 - c.x0,
      h: c.y1 - c.y0,
    }));
  }, [current]);

  async function build() {
    setBuilding(true);
    setError(null);
    setPct(0);
    try {
      const home = await api.buildTree(({ done, total }) =>
        setPct(total > 0 ? Math.round((done / total) * 100) : 0),
      );
      setTrail([home]);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("mapFail"));
    } finally {
      setBuilding(false);
    }
  }

  function open(node: TreeNode) {
    if (node.children && node.children.length > 0) setTrail([...trail, node]);
  }

  return (
    <section aria-label={t("mapTitle")}>
      <div className="group-head">
        <h2>{t("mapTitle")}</h2>
        {!building && (
          <button onClick={build}>{t("mapBuild")}</button>
        )}
      </div>

      {building && (
        <div className="progress-inline" aria-live="polite">
          <div className="progress-head">
            <span>{t("mapBuilding", { pct })}</span>
          </div>
          <div className="bar" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct}>
            <div style={{ width: `${pct}%` }} />
          </div>
        </div>
      )}

      {error && (
        <div className="alert" role="alert">
          <span>{error}</span>
        </div>
      )}

      {current && (
        <>
          <nav className="crumbs" aria-label="Caminho">
            {trail.map((n, i) => (
              <button
                key={`${n.path ?? n.name}-${i}`}
                className="crumb"
                onClick={() => setTrail(trail.slice(0, i + 1))}
                disabled={i === trail.length - 1}
              >
                {i === 0 ? t("mapRoot") : n.name}
              </button>
            ))}
            <span className="muted small crumb-size">{fmt(current.size)}</span>
          </nav>
          <p className="muted small">{t("mapHint")}</p>

          <div className="treemap">
            <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={t("mapTitle")}>
              {blocks.map((b) => {
                const clickable = !!b.node.children && b.node.children.length > 0;
                const showLabel = b.w > 90 && b.h > 40;
                return (
                  <g
                    key={b.key}
                    className={clickable ? "tm-block clickable" : "tm-block"}
                    onClick={() => open(b.node)}
                  >
                    <title>{`${b.node.name} · ${fmt(b.node.size)}`}</title>
                    <rect
                      x={b.x}
                      y={b.y}
                      width={Math.max(0, b.w)}
                      height={Math.max(0, b.h)}
                      rx={4}
                      fill={`hsl(${hueOf(b.node.name)} 38% 34%)`}
                    />
                    {showLabel && (
                      <>
                        <text x={b.x + 8} y={b.y + 18} className="tm-name">
                          {b.node.name}
                        </text>
                        <text x={b.x + 8} y={b.y + 34} className="tm-size">
                          {fmt(b.node.size)}
                        </text>
                      </>
                    )}
                  </g>
                );
              })}
            </svg>
          </div>
        </>
      )}
    </section>
  );
}
