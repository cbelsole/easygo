import type { GameView } from "../api/types";

function resultText(margin: number): string {
  if (margin === 0) return "Jigo";
  return `${margin > 0 ? "Black" : "White"} leads by ${Math.abs(margin)}`;
}

interface Props {
  view: GameView;
  busy: boolean;
  onAccept: () => void;
}

export function ScorePanel({ view, busy, onAccept }: Props) {
  const s = view.score;
  const seated = view.you.seat !== 0;
  const mySide = view.you.seat;
  const iAccepted =
    mySide === 3 ? view.accepted[0] && view.accepted[1] : mySide === 0 ? true : view.accepted[mySide - 1]!;
  return (
    <section className="panel-section">
      <h3>{view.status === "scoring" ? "Scoring" : "Score"}</h3>
      {view.status === "scoring" && (
        <p className="muted">
          {seated
            ? "Click a group to mark it dead or alive. Both players must accept the score."
            : "The players are agreeing on dead stones."}
        </p>
      )}
      {s && (
        <table className="score-table">
          <thead>
            <tr>
              <th />
              <th>Black</th>
              <th>White</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>Territory</td>
              <td>{s.blackTerritory}</td>
              <td>{s.whiteTerritory}</td>
            </tr>
            <tr>
              <td>Total (with komi and captures)</td>
              <td>{s.black}</td>
              <td>{s.white}</td>
            </tr>
          </tbody>
        </table>
      )}
      {s && <p className="score-margin">{resultText(s.margin)}</p>}
      {view.status === "scoring" && (
        <>
          <p className="muted">
            Accepted: Black {view.accepted[0] ? "yes" : "no"}, White {view.accepted[1] ? "yes" : "no"}
          </p>
          {seated && (
            <button className="btn btn-primary" disabled={busy || iAccepted} onClick={onAccept}>
              {iAccepted ? "Waiting for the other player" : "Accept score"}
            </button>
          )}
        </>
      )}
    </section>
  );
}
