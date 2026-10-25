import type { Analysis, GameView, JosekiCorner, JosekiReport, Point, ShapeFinding } from "../api/types";
import { pointLabel } from "../lib/coords";

const CORNERS: Record<JosekiCorner["corner"], string> = {
  tl: "Top left",
  tr: "Top right",
  bl: "Bottom left",
  br: "Bottom right",
};

export function AnalysisSummary({ analysis, loading }: { analysis: Analysis | null; loading: boolean }) {
  if (!analysis) return <p className="muted">{loading ? "Asking KataGo..." : "No analysis yet."}</p>;
  const black = Math.round(analysis.winrate * 1000) / 10;
  const lead = analysis.scoreLead;
  return (
    <div className="analysis">
      <div className="winbar" role="img" aria-label={`Black winrate ${black}%`}>
        <div className="winbar-b" style={{ width: `${black}%` }} />
      </div>
      <p>
        Black {black}% / White {(100 - black).toFixed(1)}%
        <br />
        Score lead: {lead >= 0 ? "Black" : "White"} {Math.abs(lead).toFixed(1)}
        <span className="muted"> ({analysis.visits} visits{loading ? ", updating" : ""})</span>
      </p>
    </div>
  );
}

export function JosekiPanel({
  report,
  size,
  mode,
}: {
  report: JosekiReport | null;
  size: number;
  mode: GameView["mode"];
}) {
  if (!report) return <p className="muted">Checking the joseki book...</p>;
  if (!report.loaded) return <p className="muted">The joseki dictionary is not loaded.</p>;
  if (size === 9) return <p className="muted">The joseki book covers 13x13 and 19x19 only.</p>;
  if (report.corners.length === 0) {
    return (
      <p className="muted">
        No corner has three or more stones yet.
        {mode === "live" ? " Play on and the book will recognize joseki." : ""}
      </p>
    );
  }
  return (
    <ul className="joseki-list">
      {report.corners.map((c) => (
        <li key={c.corner} className={c.status === "joseki" ? "joseki-ok" : "joseki-off"}>
          <strong>{CORNERS[c.corner]}:</strong>{" "}
          {c.status === "joseki"
            ? "Joseki: matched"
            : `Left the book after move ${c.deviatedAt ?? c.stones}`}
          {c.swapped && <span className="muted"> (colors swapped)</span>}
          {c.comment && <p className="comment">{c.comment}</p>}
          {c.status === "joseki" && c.next.length > 0 && (
            <p className="muted">
              Book continues:{" "}
              {c.next
                .map((n) => `${n.color === 1 ? "B" : "W"} ${pointLabel(n, size)}`)
                .join(", ")}
            </p>
          )}
        </li>
      ))}
    </ul>
  );
}

const SHAPE_NAMES: Record<ShapeFinding["kind"], string> = {
  ladder: "Ladder",
  net: "Net",
  snapback: "Snapback",
  knight: "Knight's move",
  "large-knight": "Large knight's move",
};

export function ShapesPanel({
  findings,
  size,
  onHover,
}: {
  findings: ShapeFinding[] | null;
  size: number;
  onHover: (f: ShapeFinding | null) => void;
}) {
  if (!findings) return <p className="muted">Looking for shapes...</p>;
  if (findings.length === 0) return <p className="muted">No shapes recognized here.</p>;
  return (
    <ul className="shapes-list">
      {findings.map((f, i) => (
        <li
          key={i}
          tabIndex={0}
          onMouseEnter={() => onHover(f)}
          onMouseLeave={() => onHover(null)}
          onFocus={() => onHover(f)}
          onBlur={() => onHover(null)}
        >
          <strong>{SHAPE_NAMES[f.kind]}</strong>
          {f.result && f.result !== "unknown" && (
            <span className={f.result === "works" ? "badge badge-ok" : "badge badge-bad"}>
              {f.result === "works" ? "works" : "broken"}
            </span>
          )}
          {f.confidence === "bounded" && (
            <span
              className="badge badge-warn"
              title="Read out to a limited depth. Ko and seki are not read, so the result can change."
            >
              bounded
            </span>
          )}
          <p>{f.text}</p>
          {f.breaker && <p className="muted">Breaker at {pointLabel(f.breaker, size)}</p>}
        </li>
      ))}
    </ul>
  );
}

export function shapePoints(f: ShapeFinding | null): Point[] {
  if (!f) return [];
  return [...f.points, ...(f.move ? [f.move] : [])];
}

export function StrengthLegend({ lifeDeath, strength }: { lifeDeath: boolean; strength: boolean }) {
  return (
    <ul className="legend">
      {strength && (
        <>
          <li>
            <span className="swatch tint-strong" /> strong
          </li>
          <li>
            <span className="swatch tint-moderate" /> moderate
          </li>
          <li>
            <span className="swatch tint-weak" /> weak
          </li>
        </>
      )}
      {lifeDeath && (
        <>
          <li>
            <span className="swatch swatch-dead">x</span> dead
          </li>
          <li>
            <span className="swatch swatch-seki" /> seki
          </li>
        </>
      )}
    </ul>
  );
}
