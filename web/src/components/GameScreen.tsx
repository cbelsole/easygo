import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type { AppConfig, GameView, Judgment, Point, ShapeFinding, TreeNode } from "../api/types";
import { useGame } from "../hooks/useGame";
import { useNow } from "../hooks/useNow";
import { useRemote } from "../hooks/useRemote";
import { useToast } from "../hooks/useToast";
import { indexOf, samePoint } from "../lib/coords";
import { addGhost, removeLastGhost, type Ghost } from "../lib/ghosts";
import { applyTool, type Tool } from "../lib/marks";
import {
  availability,
  groupStyles,
  needsAnalysis,
  territoryCells,
  type FeatureKey,
} from "../lib/overlays";
import { replay, totalLiberties, type Position } from "../lib/replay";
import { nodeMap, pathTo, step, type NavKey } from "../lib/tree";
import { Board } from "./Board";
import {
  AnalysisSummary,
  JosekiPanel,
  ShapesPanel,
  StrengthLegend,
  shapePoints,
} from "./HelpPanels";
import { MoveTree } from "./MoveTree";
import { NodeEditor } from "./NodeEditor";
import { PlayerCard } from "./PlayerCard";
import { ScorePanel } from "./ScorePanel";
import { FeatureBar } from "./FeatureBar";

interface Props {
  id: string;
  config: AppConfig;
  onExit: () => void;
}

export function GameScreen({ id, config, onExit }: Props) {
  const { view, receivedAt, connected, loadError, accept } = useGame(id);
  if (loadError) {
    return (
      <div className="notice notice-error">
        <p>Could not open this game: {loadError}</p>
        <button className="btn" onClick={onExit}>
          Back to lobby
        </button>
      </div>
    );
  }
  if (!view) return <p className="muted loading">Loading game...</p>;
  return (
    <GameBody
      view={view}
      receivedAt={receivedAt}
      connected={connected}
      config={config}
      accept={accept}
      onExit={onExit}
    />
  );
}

const DEFAULT_ON: Record<FeatureKey, boolean> = {
  shadow: false,
  coords: true,
  liberties: false,
  territory: false,
  liveTerritory: false,
  annotations: false,
  katago: false,
  joseki: false,
  shapes: false,
  lifeDeath: false,
  strength: false,
};

const isTyping = (t: EventTarget | null) =>
  t instanceof HTMLElement && ["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName);

function GameBody({
  view,
  receivedAt,
  connected,
  config,
  accept,
  onExit,
}: {
  view: GameView;
  receivedAt: number;
  connected: boolean;
  config: AppConfig;
  accept: (v: GameView) => void;
  onExit: () => void;
}) {
  const toast = useToast();
  const now = useNow(view.clock.some((c) => c.enabled && c.running));
  const live = view.mode === "live";
  const editable = !live && view.you.owner;
  const [on, setOn] = useState(DEFAULT_ON);
  const [viewed, setViewed] = useState<number | null>(null);
  const [ghosts, setGhosts] = useState<Ghost[]>([]);
  const [tool, setTool] = useState<Tool | null>(null);
  const [busy, setBusy] = useState(false);
  const [hoverShape, setHoverShape] = useState<ShapeFinding | null>(null);
  const [estimate, setEstimate] = useState(false);
  const [myAssist, setMyAssist] = useState(false);

  const map = useMemo(() => nodeMap(view.nodes), [view.nodes]);
  const viewedId = live && viewed !== null && map.has(viewed) ? viewed : null;
  const atHead = viewedId === null;
  const displayId = viewedId ?? view.cursor;
  const node: TreeNode | undefined = map.get(displayId);

  const pos: Position = useMemo(() => {
    if (viewedId === null) {
      return {
        board: view.board,
        liberties: view.liberties,
        captures: view.captures,
        lastMove: view.lastMove,
        toPlay: view.toPlay,
        moveNumber: view.moveNumber,
      };
    }
    const moves = pathTo(map, viewedId)
      .map((nid) => map.get(nid))
      .filter((n): n is TreeNode => n !== undefined);
    return replay(view.size, view.handicap, moves);
  }, [view, viewedId, map]);

  useEffect(() => setGhosts([]), [displayId, view.version]);
  useEffect(() => setTool(null), [view.mode, on.annotations]);

  const env = {
    mode: view.mode,
    katago: config.katago.available,
    josekiLoaded: config.joseki.loaded,
  };
  const wantAnalysis =
    estimate ||
    (Object.keys(on) as FeatureKey[]).some(
      (k) => on[k] && needsAnalysis(k) && availability(k, env).enabled,
    );
  const analysis = useRemote(
    wantAnalysis ? () => api.analysis(view.id, displayId) : null,
    `${view.id}:${displayId}:${view.version}`,
    350,
  );
  const shapes = useRemote(
    on.shapes && atHead ? () => api.shapes(view.id) : null,
    `${view.id}:${view.cursor}:${view.version}`,
  );
  const joseki = useRemote(
    on.joseki && atHead && availability("joseki", env).enabled ? () => api.joseki(view.id) : null,
    `${view.id}:${view.cursor}:${view.version}`,
  );
  const liveEnv = {
    ...env,
    katagoBlocked: analysis.refused ?? undefined,
    blocked: {
      ...(shapes.refused ? { shapes: shapes.refused } : {}),
      ...(joseki.refused ? { joseki: joseki.refused } : {}),
    },
  };
  const avail = (k: FeatureKey) => availability(k, liveEnv);
  const shown = (k: FeatureKey) => on[k] && avail(k).enabled;

  const run = useCallback(
    async (fn: () => Promise<GameView>) => {
      setBusy(true);
      try {
        accept(await fn());
      } catch (e) {
        toast.error(e);
      } finally {
        setBusy(false);
      }
    },
    [accept, toast],
  );

  const navigate = useCallback(
    (nid: number) => {
      if (live) {
        setViewed(nid === view.cursor ? null : nid);
      } else if (nid !== view.cursor) {
        void run(() => api.goto(view.id, nid));
      }
    },
    [live, view.cursor, view.id, run],
  );

  useEffect(() => {
    const keys: Record<string, NavKey> = {
      ArrowLeft: "parent",
      ArrowRight: "child",
      ArrowUp: "prevSibling",
      ArrowDown: "nextSibling",
    };
    const onKey = (e: KeyboardEvent) => {
      const k = keys[e.key];
      if (!k || isTyping(e.target) || e.altKey || e.ctrlKey || e.metaKey) return;
      const target = step(map, displayId, k);
      if (target !== undefined) {
        e.preventDefault();
        navigate(target);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [map, displayId, navigate]);

  const nextGhostColor = ((): 1 | 2 => {
    const last = ghosts[ghosts.length - 1];
    return last ? (last.color === 1 ? 2 : 1) : pos.toPlay;
  })();
  const playing = view.status === "playing";
  const canPlay = atHead && playing && view.you.canMove;
  const marking = tool !== null && editable && shown("annotations");
  const scoringClicks = view.status === "scoring" && view.you.seat !== 0;

  const onPoint = (p: Point) => {
    if (scoringClicks) {
      const isDead = view.dead.some((d) => samePoint(d, p));
      if (view.board[indexOf(p, view.size)] !== 0)
        void run(() => api.markDead(view.id, p, !isDead));
      return;
    }
    if (marking && tool) {
      const marks = applyTool(node?.marks ?? [], p, tool);
      void run(() => api.annotate(view.id, displayId, { marks }));
      return;
    }
    if (on.shadow) {
      setGhosts((g) => addGhost(g, p, pos.board, view.size, pos.toPlay));
      return;
    }
    if (canPlay) void run(() => api.play(view.id, p));
  };

  const hoverColor: 1 | 2 | undefined = marking
    ? undefined
    : on.shadow
      ? nextGhostColor
      : canPlay
        ? pos.toPlay
        : undefined;

  const settledTerritory = atHead || view.mode !== "live";
  const territory = territoryCells(
    settledTerritory ? view.territory : [],
    analysis.data?.territory,
    { settled: on.territory, live: shown("liveTerritory") },
  );
  const groups = analysis.data?.groups ?? [];
  const styles = groupStyles(groups, view.size, {
    lifeDeath: shown("lifeDeath"),
    strength: shown("strength"),
  });
  const suggestions = shown("katago") ? (analysis.data?.suggestions ?? []).slice(0, 6) : [];
  const josekiNext =
    shown("joseki") && atHead
      ? (joseki.data?.corners ?? []).flatMap((c) => (c.status === "joseki" ? c.next : []))
      : [];
  const shapeHighlight = shapePoints(hoverShape);

  const saveNode = (f: { comment: string; name: string; judgment: Judgment }) =>
    run(() => api.annotate(view.id, displayId, f));

  const seat = view.you.seat;
  const humanVsHuman = live && view.players.every((p) => p.kind === "human") && seat !== 3;
  const canUndo = (!live || !humanVsHuman) && seat !== 0 && view.moveNumber > 0 && atHead;
  const isFinished = view.status === "finished";
  const waiting = view.status === "waiting";
  const toggleOn = (k: FeatureKey) => setOn((s) => ({ ...s, [k]: !s[k] }));

  return (
    <div className="game">
      <div className="game-top">
        <button className="btn btn-quiet" onClick={onExit}>
          Lobby
        </button>
        <h2 className="game-title">{view.name}</h2>
        <span className="chip-info">{modeName(view.mode)}</span>
        <span className="chip-info">
          {view.size}x{view.size}, {view.rules.preset ?? "custom"}, komi {view.rules.komi}
          {view.handicap > 1 ? `, handicap ${view.handicap}` : ""}
        </span>
        {!connected && <span className="chip-info chip-warn">Reconnecting...</span>}
      </div>

      {isFinished && (
        <div className="banner banner-result" role="status">
          <strong>Game over{view.result ? `: ${view.result}` : ""}</strong>
          <a className="btn" href={api.sgfUrl(view.id)} download>
            Download SGF
          </a>
        </div>
      )}
      {waiting && (
        <div className="banner" role="status">
          {seat === 0 ? (
            <>
              <span>This game has an open seat.</span>
              <button
                className="btn btn-primary"
                disabled={busy}
                onClick={() => void run(() => api.join(view.id))}
              >
                Join game
              </button>
            </>
          ) : (
            <span>Waiting for an opponent to join. Share this page&apos;s address with them.</span>
          )}
        </div>
      )}

      <PlayerCard
        color={2}
        player={view.players[1]}
        clock={view.clock[1]}
        captures={pos.captures[1]}
        active={playing && pos.toPlay === 2 && atHead}
        receivedAt={receivedAt}
        now={now}
      />

      <div className="game-main">
        <div className="board-wrap">
          <Board
            size={view.size}
            stones={pos.board}
            lastMove={pos.lastMove}
            showCoords={on.coords}
            liberties={on.liberties ? pos.liberties : undefined}
            territory={territory}
            groupStyles={styles}
            dead={view.status === "scoring" || isFinished ? view.dead : undefined}
            marks={node?.marks ?? []}
            ghosts={ghosts}
            suggestions={suggestions}
            suggestionFor={pos.toPlay}
            josekiNext={josekiNext}
            highlight={shapeHighlight}
            breaker={hoverShape?.breaker}
            hoverColor={hoverColor}
            clickable={scoringClicks || marking || on.shadow || canPlay}
            onPoint={onPoint}
            onRightClick={() => setGhosts((g) => removeLastGhost(g))}
          />
        </div>

        <aside className="side" aria-label="Game panel">
          <section className="panel-section">
            <h3>
              Move {pos.moveNumber}
              {playing && (
                <span className="muted"> - {pos.toPlay === 1 ? "Black" : "White"} to play</span>
              )}
            </h3>
            <p className="muted">
              Libs: B {totalLiberties(pos.board, view.size, 1)} W{" "}
              {totalLiberties(pos.board, view.size, 2)}
            </p>
            {live && !atHead && (
              <div className="banner banner-history" role="status">
                <span>Reviewing move {pos.moveNumber}.</span>
                <button className="btn btn-primary" onClick={() => setViewed(null)}>
                  Back to live
                </button>
              </div>
            )}
            <MoveTree nodes={view.nodes} size={view.size} current={displayId} onSelect={navigate} />
            <div className="row">
              {(
                [
                  ["parent", "Previous", "←"],
                  ["child", "Next", "→"],
                  ["prevSibling", "Previous variation", "↑"],
                  ["nextSibling", "Next variation", "↓"],
                ] as const
              ).map(([k, label, glyph]) => (
                <button
                  key={k}
                  className="btn btn-icon"
                  aria-label={label}
                  title={label}
                  disabled={step(map, displayId, k) === undefined}
                  onClick={() => {
                    const t = step(map, displayId, k);
                    if (t !== undefined) navigate(t);
                  }}
                >
                  {glyph}
                </button>
              ))}
              {editable && node && node.color !== 0 && (
                <>
                  <button
                    className="btn"
                    disabled={busy}
                    onClick={() => void run(() => api.promote(view.id, displayId))}
                  >
                    Promote branch
                  </button>
                  <button
                    className="btn btn-danger"
                    disabled={busy}
                    onClick={() => void run(() => api.deleteNode(view.id, displayId))}
                  >
                    Delete branch
                  </button>
                </>
              )}
            </div>
            {view.partial && (
              <p className="muted">
                Large tree: only nearby moves are loaded. Step into a branch to load more.
              </p>
            )}
          </section>

          {node && <NodeEditor node={node} editable={editable} onSave={(f) => void saveNode(f)} />}

          {(isFinished || view.status === "scoring") && (
            <ScorePanel
              view={view}
              busy={busy}
              onAccept={() => void run(() => api.accept(view.id))}
            />
          )}

          {humanVsHuman && playing && (
            <section className="panel-section">
              <h3>AI help</h3>
              <label className="check">
                <input
                  type="checkbox"
                  checked={myAssist}
                  onChange={(e) => {
                    setMyAssist(e.target.checked);
                    void run(() => api.assist(view.id, e.target.checked));
                  }}
                />
                Allow AI help
              </label>
              <p className="muted">
                KataGo help is {view.aiAssist ? "on" : "off"}. Both players must agree before it is
                available.
              </p>
            </section>
          )}

          {estimate && (
            <section className="panel-section">
              <h3>Score estimate</h3>
              {view.score && (
                <p>
                  Settled territory: Black {view.score.black}, White {view.score.white} (
                  {view.score.margin > 0 ? "Black" : "White"} {Math.abs(view.score.margin)})
                </p>
              )}
              {avail("katago").enabled ? (
                <AnalysisSummary analysis={analysis.data} loading={analysis.loading} />
              ) : (
                <p className="muted">KataGo estimate unavailable: {avail("katago").reason}</p>
              )}
            </section>
          )}

          {shown("annotations") && editable && (
            <section className="panel-section">
              <h3>Annotation tools</h3>
              <ToolPalette tool={tool} onChange={setTool} />
            </section>
          )}

          {on.katago && (
            <section className="panel-section">
              <h3>KataGo</h3>
              {avail("katago").enabled ? (
                <AnalysisSummary analysis={analysis.data} loading={analysis.loading} />
              ) : (
                <p className="muted">{avail("katago").reason}</p>
              )}
            </section>
          )}

          {(on.lifeDeath || on.strength) && (
            <section className="panel-section">
              <h3>Groups</h3>
              {shown("lifeDeath") || shown("strength") ? (
                <StrengthLegend lifeDeath={shown("lifeDeath")} strength={shown("strength")} />
              ) : (
                <p className="muted">{avail(on.lifeDeath ? "lifeDeath" : "strength").reason}</p>
              )}
            </section>
          )}

          {on.liveTerritory && !shown("liveTerritory") && (
            <p className="muted">Live territory: {avail("liveTerritory").reason}</p>
          )}

          {on.joseki && (
            <section className="panel-section">
              <h3>Joseki</h3>
              {avail("joseki").enabled ? (
                atHead ? (
                  <JosekiPanel report={joseki.data} size={view.size} mode={view.mode} />
                ) : (
                  <p className="muted">Shown for the live position.</p>
                )
              ) : (
                <p className="muted">{avail("joseki").reason}</p>
              )}
            </section>
          )}

          {on.shapes && (
            <section className="panel-section">
              <h3>Shapes</h3>
              {shapes.refused ? (
                <p className="muted">{shapes.refused}</p>
              ) : atHead ? (
                <ShapesPanel findings={shapes.data} size={view.size} onHover={setHoverShape} />
              ) : (
                <p className="muted">Shown for the live position.</p>
              )}
            </section>
          )}
        </aside>
      </div>

      <PlayerCard
        color={1}
        player={view.players[0]}
        clock={view.clock[0]}
        captures={pos.captures[0]}
        active={playing && pos.toPlay === 1 && atHead}
        receivedAt={receivedAt}
        now={now}
      />

      <div className="bar">
        <div className="bar-actions">
          <button
            className="btn"
            disabled={busy || !canPlay}
            onClick={() => void run(() => api.pass(view.id))}
          >
            Pass
          </button>
          <button
            className="btn btn-danger"
            disabled={busy || !(live && playing && seat !== 0)}
            onClick={() => {
              if (window.confirm("Resign this game?")) void run(() => api.resign(view.id));
            }}
          >
            Resign
          </button>
          <button
            className="btn"
            disabled={busy || !canUndo}
            onClick={() => void run(() => api.undo(view.id))}
          >
            Undo
          </button>
          <button className="btn" aria-pressed={estimate} onClick={() => setEstimate((e) => !e)}>
            Score estimate
          </button>
        </div>
        <FeatureBar mode={view.mode} on={on} avail={avail} onToggle={toggleOn} />
      </div>
    </div>
  );
}

function modeName(m: GameView["mode"]): string {
  return m === "live" ? "Live game" : m === "teaching" ? "Teaching game" : "Joseki example";
}

const TOOLS: { id: Tool; label: string; glyph: string }[] = [
  { id: "letters", label: "Letters A, B, C", glyph: "A" },
  { id: "numbers", label: "Numbers 1, 2, 3", glyph: "1" },
  { id: "triangle", label: "Triangle", glyph: "△" },
  { id: "square", label: "Square", glyph: "□" },
  { id: "circle", label: "Circle", glyph: "○" },
  { id: "cross", label: "Cross", glyph: "✕" },
];

function ToolPalette({
  tool,
  onChange,
}: {
  tool: Tool | null;
  onChange: (t: Tool | null) => void;
}) {
  return (
    <div className="row" role="toolbar" aria-label="Annotation tools">
      {TOOLS.map((t) => (
        <button
          key={t.id}
          className="btn btn-icon"
          aria-pressed={tool === t.id}
          aria-label={t.label}
          title={t.label}
          onClick={() => onChange(tool === t.id ? null : t.id)}
        >
          {t.glyph}
        </button>
      ))}
    </div>
  );
}
