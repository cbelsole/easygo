import type { Mode } from "../api/types";
import type { Availability, FeatureKey } from "../lib/overlays";

const FEATURES: { key: FeatureKey; label: string }[] = [
  { key: "shadow", label: "Shadow moves" },
  { key: "coords", label: "Coordinates" },
  { key: "liberties", label: "Liberties" },
  { key: "territory", label: "Territory" },
  { key: "liveTerritory", label: "Live territory" },
  { key: "annotations", label: "Annotations" },
  { key: "katago", label: "Next move" },
  { key: "joseki", label: "Joseki" },
  { key: "shapes", label: "Shapes" },
  { key: "lifeDeath", label: "Life and death" },
  { key: "strength", label: "Strong/weak" },
];

interface Props {
  mode: Mode;
  on: Record<FeatureKey, boolean>;
  avail: (k: FeatureKey) => Availability;
  onToggle: (k: FeatureKey) => void;
}

export function FeatureBar({ mode, on, avail, onToggle }: Props) {
  return (
    <div className="toggles" role="group" aria-label="Board features">
      {FEATURES.filter((f) => !(f.key === "annotations" && mode === "live")).map((f) => {
        const a = avail(f.key);
        const disabled = !a.enabled && !on[f.key];
        return (
          <button
            key={f.key}
            className={`toggle ${on[f.key] ? "toggle-on" : ""} ${!a.enabled ? "toggle-grey" : ""}`}
            aria-pressed={on[f.key]}
            disabled={disabled}
            title={a.enabled ? undefined : a.reason}
            onClick={() => onToggle(f.key)}
          >
            {f.label}
          </button>
        );
      })}
    </div>
  );
}
