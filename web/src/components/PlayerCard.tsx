import type { ClockView, PlayerView } from "../api/types";
import { formatClock, isLow, readClock } from "../lib/clock";

interface Props {
  color: 1 | 2;
  player: PlayerView;
  clock: ClockView;
  captures: number;
  active: boolean;
  receivedAt: number;
  now: number;
}

export function PlayerCard({ color, player, clock, captures, active, receivedAt, now }: Props) {
  const side = color === 1 ? "Black" : "White";
  const reading = readClock(clock, now - receivedAt);
  const total = clock.config.periods;
  const name = player.kind === "open" ? "Waiting for a player" : player.name;

  return (
    <section
      className={`card ${active ? "card-active" : ""} ${player.you ? "card-you" : ""}`}
      aria-label={`${side} player`}
    >
      <span className={`card-stone ${color === 1 ? "stone-b" : "stone-w"}`} aria-hidden="true" />
      <div className="card-who">
        <div className="card-name">
          <strong>{name}</strong>
          {player.you && <span className="badge">you</span>}
          {player.kind === "ai" && <span className="badge badge-ai">KataGo</span>}
          {active && <span className="turn-dot" title="To play" aria-label="To play" />}
        </div>
        <div className="card-sub">
          {[player.rank, player.location].filter(Boolean).join(" - ") || side}
        </div>
      </div>
      <div className="card-caps" title="Captured stones">
        <span className="caps-n">{captures}</span>
        <span className="caps-l">captures</span>
      </div>
      {clock.enabled && (
        <div
          className={`clock ${isLow(reading) ? "clock-low" : ""} ${reading.flagged ? "clock-flagged" : ""}`}
          aria-label={`${side} clock`}
        >
          <span className="clock-time">{formatClock(reading)}</span>
          {reading.inByo && <span className="clock-phase">byo-yomi</span>}
          {clock.byoConfigured && total > 0 && (
            <span
              className="periods"
              role="img"
              aria-label={`${reading.periods} of ${total} periods left`}
            >
              {Array.from({ length: total }, (_, i) => (
                <span key={i} className={i < reading.periods ? "dot dot-on" : "dot"} />
              ))}
            </span>
          )}
        </div>
      )}
    </section>
  );
}
