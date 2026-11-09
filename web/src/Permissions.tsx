import { useEffect } from "react";
import type { Key, T } from "./i18n";

const ITEMS: { n: number }[] = [{ n: 1 }, { n: 2 }, { n: 3 }, { n: 4 }, { n: 5 }];

export default function Permissions({ onClose, t }: { onClose: () => void; t: T }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="perm-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="modal-head">
          <h2 id="perm-title">{t("perms")}</h2>
          <button onClick={onClose} aria-label={t("close")}>
            {t("close")}
          </button>
        </div>
        <ul className="perm-list">
          {ITEMS.map(({ n }) => {
            const title = t(`p${n}t` as Key);
            return (
              <li key={n} className="perm">
                <strong>{title}</strong>
                <span className="muted small">{t(`p${n}w` as Key)}</span>
                <span className="small">
                  {t("usedIn")} {t(`p${n}u` as Key)}
                </span>
              </li>
            );
          })}
        </ul>
      </div>
    </div>
  );
}
