import { useId, useMemo, useState, type MouseEvent, type PointerEvent } from "react";
import type { Color, JosekiNext, Mark, Point, Suggestion } from "../api/types";
import { columnLabel, indexOf, rowLabel, samePoint, starPoints } from "../lib/coords";
import type { Ghost } from "../lib/ghosts";
import type { GroupStyle } from "../lib/overlays";

export interface BoardProps {
  size: number;
  stones: Color[];
  lastMove?: Point;
  showCoords: boolean;
  liberties?: number[];
  /** 0 neutral, 1 black, 2 white, per point. */
  territory?: number[];
  groupStyles?: Map<number, GroupStyle>;
  /** Stones currently marked dead in the scoring phase. */
  dead?: Point[];
  marks: Mark[];
  ghosts: Ghost[];
  suggestions?: Suggestion[];
  /** Whether suggestion numbers are for black or white (the side to move). */
  suggestionFor?: 1 | 2;
  josekiNext?: JosekiNext[];
  highlight?: Point[];
  breaker?: Point;
  /** Color of the preview stone shown under the pointer; undefined disables hover and clicks. */
  hoverColor?: 1 | 2;
  /** Clicks are active even without a hover preview (scoring, annotation). */
  clickable: boolean;
  onPoint: (p: Point) => void;
  onRightClick: (p: Point) => void;
}

const STONE_R = 0.47;
// Distance from the stone edge to the centre line of the strong/weak ring.
const TINT_RING_OFFSET = 0.1;

export function Board(props: BoardProps) {
  const { size, stones, showCoords } = props;
  const uid = useId().replace(/:/g, "");
  const [hover, setHover] = useState<Point | null>(null);
  const pad = showCoords ? 1.15 : 0.6;
  const min = -pad;
  const span = size - 1 + 2 * pad;
  const stars = useMemo(() => starPoints(size), [size]);
  const deadSet = useMemo(
    () => new Set((props.dead ?? []).map((p) => indexOf(p, size))),
    [props.dead, size],
  );
  const toPoint = (e: {
    clientX: number;
    clientY: number;
    currentTarget: Element;
  }): Point | null => {
    const r = e.currentTarget.getBoundingClientRect();
    const x = Math.round(min + ((e.clientX - r.left) / r.width) * span);
    const y = Math.round(min + ((e.clientY - r.top) / r.height) * span);
    return x >= 0 && y >= 0 && x < size && y < size ? { x, y } : null;
  };

  const onMove = (e: PointerEvent<SVGRectElement>) => {
    const p = toPoint(e);
    setHover((h) => (p && h && samePoint(p, h) ? h : p));
  };
  const onClick = (e: MouseEvent<SVGRectElement>) => {
    const p = toPoint(e);
    if (p && props.clickable) props.onPoint(p);
  };
  const onContext = (e: MouseEvent<SVGRectElement>) => {
    e.preventDefault();
    const p = toPoint(e);
    if (p) props.onRightClick(p);
  };

  const ticks = Array.from({ length: size }, (_, i) => i);
  const showHover =
    props.hoverColor !== undefined && hover !== null && stones[indexOf(hover, size)] === 0;

  return (
    <svg
      className="board"
      viewBox={`${min} ${min} ${span} ${span}`}
      role="img"
      aria-label={`Go board ${size} by ${size}`}
    >
      <defs>
        <radialGradient id={`${uid}b`} cx="35%" cy="30%" r="75%">
          <stop offset="0%" stopColor="#6a6a6a" />
          <stop offset="55%" stopColor="#1c1c1c" />
          <stop offset="100%" stopColor="#050505" />
        </radialGradient>
        <radialGradient id={`${uid}w`} cx="35%" cy="30%" r="80%">
          <stop offset="0%" stopColor="#ffffff" />
          <stop offset="70%" stopColor="#e6e6e6" />
          <stop offset="100%" stopColor="#b9b9b9" />
        </radialGradient>
      </defs>
      <rect className="board-surface" x={min} y={min} width={span} height={span} rx={0.25} />

      <g className="board-grid">
        {ticks.map((i) => (
          <g key={i}>
            <line x1={0} y1={i} x2={size - 1} y2={i} />
            <line x1={i} y1={0} x2={i} y2={size - 1} />
          </g>
        ))}
        {stars.map((s) => (
          <circle key={`${s.x}-${s.y}`} className="board-star" cx={s.x} cy={s.y} r={0.11} />
        ))}
      </g>

      {showCoords && (
        <g className="board-coords" aria-hidden="true">
          {ticks.map((i) => (
            <g key={i}>
              <text x={i} y={min + 0.62} textAnchor="middle">
                {columnLabel(i)}
              </text>
              <text x={i} y={size - 1 + pad - 0.28} textAnchor="middle">
                {columnLabel(i)}
              </text>
              <text x={min + 0.55} y={i + 0.13} textAnchor="middle">
                {rowLabel(i, size)}
              </text>
              <text x={size - 1 + pad - 0.55} y={i + 0.13} textAnchor="middle">
                {rowLabel(i, size)}
              </text>
            </g>
          ))}
        </g>
      )}

      {props.territory && (
        <g className="territory">
          {props.territory.map((t, i) => {
            if (t !== 1 && t !== 2) return null;
            if (stones[i] !== 0 && !deadSet.has(i)) return null;
            return (
              <rect
                key={i}
                className={t === 1 ? "terr-black" : "terr-white"}
                x={(i % size) - 0.17}
                y={Math.floor(i / size) - 0.17}
                width={0.34}
                height={0.34}
              />
            );
          })}
        </g>
      )}

      {props.highlight && props.highlight.length > 0 && (
        <g className="highlight">
          {props.highlight.map((p) => (
            <circle key={`${p.x}-${p.y}`} cx={p.x} cy={p.y} r={0.58} />
          ))}
        </g>
      )}

      {/* Strong/weak rings sit below every stone so one never paints over a neighbour. */}
      <g>
        {stones.map((c, i) => {
          const style = props.groupStyles?.get(i);
          if (c === 0 || !style?.tint || deadSet.has(i) || style.status === "dead") return null;
          return (
            <circle
              key={i}
              className={`tint tint-${style.strength}`}
              cx={i % size}
              cy={Math.floor(i / size)}
              r={STONE_R + TINT_RING_OFFSET}
            />
          );
        })}
      </g>

      <g>
        {stones.map((c, i) => {
          if (c === 0) return null;
          const x = i % size;
          const y = Math.floor(i / size);
          const style = props.groupStyles?.get(i);
          const isDead = deadSet.has(i) || style?.status === "dead";
          return (
            <g key={i} opacity={isDead ? 0.4 : 1}>
              {style?.status === "seki" && (
                <circle className="seki-ring" cx={x} cy={y} r={STONE_R + 0.12} />
              )}
              <circle className="stone-shadow" cx={x + 0.05} cy={y + 0.07} r={STONE_R} />
              <circle
                className={c === 1 ? "stone stone-black" : "stone stone-white"}
                cx={x}
                cy={y}
                r={STONE_R}
                fill={`url(#${uid}${c === 1 ? "b" : "w"})`}
              />
              {isDead && (
                <path
                  className="dead-x"
                  d={`M${x - 0.2} ${y - 0.2}L${x + 0.2} ${y + 0.2}M${x + 0.2} ${y - 0.2}L${x - 0.2} ${y + 0.2}`}
                />
              )}
            </g>
          );
        })}
      </g>

      {props.liberties && (
        <g className="liberties" aria-hidden="true">
          {props.liberties.map((n, i) => {
            const c = stones[i];
            if (!n || !c) return null;
            return (
              <text
                key={i}
                className={c === 1 ? "on-black" : "on-white"}
                x={i % size}
                y={Math.floor(i / size) + 0.18}
                textAnchor="middle"
              >
                {n}
              </text>
            );
          })}
        </g>
      )}

      {props.lastMove && (
        <circle
          className={`last-move ${stones[indexOf(props.lastMove, size)] === 1 ? "on-black" : "on-white"}`}
          cx={props.lastMove.x}
          cy={props.lastMove.y}
          r={0.22}
        />
      )}

      {props.breaker && (
        <circle className="breaker" cx={props.breaker.x} cy={props.breaker.y} r={0.62} />
      )}

      <MarkLayer marks={props.marks} stones={stones} size={size} />
      <JosekiLayer next={props.josekiNext ?? []} />
      <SuggestionLayer suggestions={props.suggestions ?? []} toPlay={props.suggestionFor ?? 1} />

      {props.ghosts.map((g) => (
        <g key={`${g.x}-${g.y}`} className="ghost" opacity={0.5}>
          <circle
            className={g.color === 1 ? "stone stone-black" : "stone stone-white"}
            cx={g.x}
            cy={g.y}
            r={STONE_R}
            fill={`url(#${uid}${g.color === 1 ? "b" : "w"})`}
          />
        </g>
      ))}

      {showHover && hover && props.hoverColor && (
        <circle
          className={props.hoverColor === 1 ? "stone stone-black" : "stone stone-white"}
          cx={hover.x}
          cy={hover.y}
          r={STONE_R}
          opacity={0.45}
          fill={`url(#${uid}${props.hoverColor === 1 ? "b" : "w"})`}
          pointerEvents="none"
        />
      )}

      <rect
        className={props.clickable ? "board-hit board-hit-active" : "board-hit"}
        x={min}
        y={min}
        width={span}
        height={span}
        fill="transparent"
        onPointerMove={onMove}
        onPointerLeave={() => setHover(null)}
        onClick={onClick}
        onContextMenu={onContext}
      />
    </svg>
  );
}

function MarkLayer({ marks, stones, size }: { marks: Mark[]; stones: Color[]; size: number }) {
  return (
    <g className="marks" pointerEvents="none">
      {marks.map((m) => {
        const under = stones[indexOf(m, size)];
        const tone = under === 1 ? "on-black" : under === 2 ? "on-white" : "on-empty";
        const r = 0.27;
        return (
          <g key={`${m.x}-${m.y}-${m.kind}`} className={`mark ${tone}`}>
            {under === 0 && <circle className="mark-bg" cx={m.x} cy={m.y} r={0.36} />}
            {m.kind === "label" && (
              <text x={m.x} y={m.y + 0.2} textAnchor="middle">
                {m.text}
              </text>
            )}
            {m.kind === "triangle" && (
              <path
                d={`M${m.x} ${m.y - r}L${m.x + r} ${m.y + r * 0.8}L${m.x - r} ${m.y + r * 0.8}Z`}
              />
            )}
            {m.kind === "square" && (
              <rect x={m.x - r * 0.85} y={m.y - r * 0.85} width={r * 1.7} height={r * 1.7} />
            )}
            {m.kind === "circle" && <circle cx={m.x} cy={m.y} r={r} />}
            {m.kind === "cross" && (
              <path
                d={`M${m.x - r} ${m.y - r}L${m.x + r} ${m.y + r}M${m.x + r} ${m.y - r}L${m.x - r} ${m.y + r}`}
              />
            )}
          </g>
        );
      })}
    </g>
  );
}

function JosekiLayer({ next }: { next: JosekiNext[] }) {
  return (
    <g className="joseki-next" pointerEvents="none">
      {next.map((n) => (
        <g key={`${n.x}-${n.y}-${n.color}`}>
          <circle className="joseki-outer" cx={n.x} cy={n.y} r={0.4} />
          <circle
            className={n.color === 1 ? "joseki-black" : "joseki-white"}
            cx={n.x}
            cy={n.y}
            r={0.4}
          />
        </g>
      ))}
    </g>
  );
}

function SuggestionLayer({ suggestions, toPlay }: { suggestions: Suggestion[]; toPlay: 1 | 2 }) {
  if (suggestions.length === 0) return null;
  const maxVisits = Math.max(...suggestions.map((s) => s.visits), 1);
  return (
    <g className="suggestions" pointerEvents="none">
      {suggestions.map((s, i) => {
        if (s.pass) return null;
        const mine = toPlay === 1 ? s.winrate : 1 - s.winrate;
        const lead = toPlay === 1 ? s.scoreLead : -s.scoreLead;
        const strength = 0.3 + 0.5 * (s.visits / maxVisits);
        return (
          <g key={`${s.x}-${s.y}`} className={i === 0 ? "suggest suggest-best" : "suggest"}>
            <circle cx={s.x} cy={s.y} r={0.46} fillOpacity={strength} />
            <text x={s.x} y={s.y - 0.02} textAnchor="middle" className="s-win">
              {Math.round(mine * 100)}
            </text>
            <text x={s.x} y={s.y + 0.3} textAnchor="middle" className="s-lead">
              {lead >= 0 ? "+" : ""}
              {lead.toFixed(1)}
            </text>
          </g>
        );
      })}
    </g>
  );
}
