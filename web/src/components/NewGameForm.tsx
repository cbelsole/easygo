import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import type {
  AppConfig,
  ColorChoice,
  GameView,
  Mode,
  NewGameRequest,
  Opponent,
  Rules,
} from "../api/types";
import { useToast } from "../hooks/useToast";

interface Props {
  config: AppConfig;
  onCreated: (g: GameView) => void;
}

const STRENGTHS = [
  { visits: 10, label: "Beginner (10 visits)" },
  { visits: 50, label: "Easy (50 visits)" },
  { visits: 200, label: "Medium (200 visits)" },
  { visits: 800, label: "Strong (800 visits)" },
  { visits: 3000, label: "Very strong (3000 visits)" },
];

const MODES: { id: Mode; label: string; help: string }[] = [
  { id: "live", label: "Live Game", help: "Play a game with clocks and scoring." },
  { id: "teaching", label: "Teaching Game", help: "Free navigation, branches, comments and marks." },
  { id: "joseki", label: "Joseki Example", help: "Build an example sequence you can annotate." },
];

const stripPreset = ({ preset: _preset, ...rest }: Rules): Rules => rest;
const sameRules = (a: Rules, b: Rules) =>
  JSON.stringify(stripPreset(a)) === JSON.stringify(stripPreset(b));

export function NewGameForm({ config, onCreated }: Props) {
  const toast = useToast();
  const firstPreset = config.presets[0];
  const [mode, setMode] = useState<Mode>("live");
  const [name, setName] = useState("");
  const [size, setSize] = useState(config.sizes.includes(19) ? 19 : (config.sizes[0] ?? 19));
  const [preset, setPreset] = useState(firstPreset?.name ?? "japanese");
  const [rules, setRules] = useState<Rules | undefined>(firstPreset?.rules);
  const [handicap, setHandicap] = useState(0);
  const [opponent, setOpponent] = useState<Opponent>("local");
  const [color, setColor] = useState<ColorChoice>("black");
  const [visits, setVisits] = useState(200);
  const [useClock, setUseClock] = useState(false);
  const [mainMinutes, setMainMinutes] = useState(10);
  const [periods, setPeriods] = useState(5);
  const [periodSeconds, setPeriodSeconds] = useState(30);
  const [busy, setBusy] = useState(false);

  const katago = config.katago.available;
  const live = mode === "live";
  const presetRules = config.presets.find((p) => p.name === preset)?.rules;

  const choosePreset = (p: string) => {
    setPreset(p);
    setRules(config.presets.find((x) => x.name === p)?.rules);
  };
  const setRule = <K extends keyof Rules>(k: K, v: Rules[K]) =>
    setRules((r) => (r ? { ...r, [k]: v } : r));

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const custom = rules && presetRules && !sameRules(rules, presetRules);
    const req: NewGameRequest = {
      name: name.trim() || defaultName(mode),
      mode,
      size,
      rules: custom && rules ? stripPreset(rules) : preset,
      handicap,
      opponent: live ? opponent : "local",
      color: live && opponent !== "local" ? color : "black",
      aiVisits: visits,
      clock: {
        enabled: useClock && mode !== "joseki",
        mainSeconds: Math.max(0, Math.round(mainMinutes * 60)),
        periods: Math.max(0, Math.round(periods)),
        periodSeconds: Math.max(0, Math.round(periodSeconds)),
      },
    };
    setBusy(true);
    try {
      onCreated(await api.createGame(req));
    } catch (err) {
      toast.error(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="panel form" onSubmit={(e) => void submit(e)}>
      <h2>New game</h2>

      <fieldset>
        <legend>Mode</legend>
        <div className="segmented" role="radiogroup" aria-label="Mode">
          {MODES.map((m) => (
            <label key={m.id} className={mode === m.id ? "seg-on" : ""}>
              <input
                type="radio"
                name="mode"
                checked={mode === m.id}
                onChange={() => setMode(m.id)}
              />
              {m.label}
            </label>
          ))}
        </div>
        <p className="muted">{MODES.find((m) => m.id === mode)?.help}</p>
      </fieldset>

      <label className="field">
        <span>Name</span>
        <input value={name} placeholder={defaultName(mode)} onChange={(e) => setName(e.target.value)} />
      </label>

      <div className="grid2">
        <label className="field">
          <span>Board size</span>
          <select value={size} onChange={(e) => setSize(Number(e.target.value))}>
            {config.sizes.map((s) => (
              <option key={s} value={s}>
                {s} x {s}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Rules</span>
          <select value={preset} onChange={(e) => choosePreset(e.target.value)}>
            {config.presets.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      <details className="advanced">
        <summary>Advanced rules</summary>
        {rules && (
          <div className="grid2">
            <label className="field">
              <span>Ko</span>
              <select value={rules.ko} onChange={(e) => setRule("ko", e.target.value as Rules["ko"])}>
                <option value="SIMPLE">Simple</option>
                <option value="POSITIONAL">Positional superko</option>
                <option value="SITUATIONAL">Situational superko</option>
              </select>
            </label>
            <label className="field">
              <span>Scoring</span>
              <select
                value={rules.scoring}
                onChange={(e) => setRule("scoring", e.target.value as Rules["scoring"])}
              >
                <option value="TERRITORY">Territory</option>
                <option value="AREA">Area</option>
              </select>
            </label>
            <label className="field">
              <span>Tax</span>
              <select value={rules.tax} onChange={(e) => setRule("tax", e.target.value as Rules["tax"])}>
                <option value="NONE">None</option>
                <option value="SEKI">Seki</option>
                <option value="ALL">All</option>
              </select>
            </label>
            <label className="field">
              <span>White handicap bonus</span>
              <select
                value={rules.whiteHandicapBonus}
                onChange={(e) =>
                  setRule("whiteHandicapBonus", e.target.value as Rules["whiteHandicapBonus"])
                }
              >
                <option value="0">None</option>
                <option value="N-1">Stones minus one</option>
                <option value="N">Number of stones</option>
              </select>
            </label>
            <label className="field">
              <span>Komi</span>
              <input
                type="number"
                step="0.5"
                value={rules.komi}
                onChange={(e) => setRule("komi", Number(e.target.value))}
              />
            </label>
            <div className="field">
              <label className="check">
                <input
                  type="checkbox"
                  checked={rules.suicide}
                  onChange={(e) => setRule("suicide", e.target.checked)}
                />
                Multi-stone suicide allowed
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={rules.hasButton}
                  onChange={(e) => setRule("hasButton", e.target.checked)}
                />
                Button (first pass is worth half a point)
              </label>
            </div>
          </div>
        )}
      </details>

      <label className="field">
        <span>Handicap</span>
        <select value={handicap} onChange={(e) => setHandicap(Number(e.target.value))}>
          <option value={0}>None</option>
          {[2, 3, 4, 5, 6, 7, 8, 9].map((h) => (
            <option key={h} value={h}>
              {h} stones
            </option>
          ))}
        </select>
      </label>

      {live && (
        <fieldset>
          <legend>Opponent</legend>
          <div className="segmented" role="radiogroup" aria-label="Opponent">
            <label className={opponent === "local" ? "seg-on" : ""}>
              <input
                type="radio"
                name="opp"
                checked={opponent === "local"}
                onChange={() => setOpponent("local")}
              />
              Friend on this screen
            </label>
            <label className={opponent === "ai" ? "seg-on" : ""} title={katago ? undefined : "KataGo is not running"}>
              <input
                type="radio"
                name="opp"
                disabled={!katago}
                checked={opponent === "ai"}
                onChange={() => setOpponent("ai")}
              />
              KataGo
            </label>
            <label className={opponent === "open" ? "seg-on" : ""}>
              <input
                type="radio"
                name="opp"
                checked={opponent === "open"}
                onChange={() => setOpponent("open")}
              />
              Open seat for another player
            </label>
          </div>
          {!katago && (
            <p className="muted">
              KataGo is not running{config.katago.error ? ` (${config.katago.error})` : ""}. Set its paths in
              Settings below.
            </p>
          )}
          {opponent !== "local" && (
            <label className="field">
              <span>My color</span>
              <select value={color} onChange={(e) => setColor(e.target.value as ColorChoice)}>
                <option value="black">Black</option>
                <option value="white">White</option>
                <option value="random">Random</option>
              </select>
            </label>
          )}
          {opponent === "ai" && (
            <label className="field">
              <span>KataGo strength</span>
              <select value={visits} onChange={(e) => setVisits(Number(e.target.value))}>
                {STRENGTHS.map((s) => (
                  <option key={s.visits} value={s.visits}>
                    {s.label}
                  </option>
                ))}
              </select>
            </label>
          )}
        </fieldset>
      )}

      {mode !== "joseki" && (
        <fieldset>
          <legend>Clock</legend>
          <label className="check">
            <input type="checkbox" checked={useClock} onChange={(e) => setUseClock(e.target.checked)} />
            Use a clock
          </label>
          <div className="grid3" aria-disabled={!useClock}>
            <label className="field">
              <span>Main time (minutes, 0 = none)</span>
              <input
                type="number"
                min={0}
                disabled={!useClock}
                value={mainMinutes}
                onChange={(e) => setMainMinutes(Number(e.target.value))}
              />
            </label>
            <label className="field">
              <span>Byo-yomi periods (0 = none)</span>
              <input
                type="number"
                min={0}
                disabled={!useClock}
                value={periods}
                onChange={(e) => setPeriods(Number(e.target.value))}
              />
            </label>
            <label className="field">
              <span>Seconds per period</span>
              <input
                type="number"
                min={1}
                disabled={!useClock || periods === 0}
                value={periodSeconds}
                onChange={(e) => setPeriodSeconds(Number(e.target.value))}
              />
            </label>
          </div>
        </fieldset>
      )}

      <button className="btn btn-primary" type="submit" disabled={busy}>
        {busy ? "Creating..." : "Start game"}
      </button>
    </form>
  );
}

function defaultName(mode: Mode): string {
  return mode === "live" ? "Live game" : mode === "teaching" ? "Teaching game" : "Joseki example";
}
