import { useEffect, useMemo, useRef } from "react";
import type { TreeNode } from "../api/types";
import { pointLabel } from "../lib/coords";
import {
  buildSegments,
  hasUnloadedChildren,
  nodeMap,
  rootId,
  type NodeMap,
  type Segment,
} from "../lib/tree";

interface Props {
  nodes: TreeNode[];
  size: number;
  current: number;
  onSelect: (id: number) => void;
}

function nodeText(n: TreeNode, size: number): string {
  if (n.color === 0) return "Start";
  return n.pass ? "Pass" : pointLabel(n, size);
}

export function MoveTree({ nodes, size, current, onSelect }: Props) {
  const map = useMemo(() => nodeMap(nodes), [nodes]);
  const root = useMemo(() => rootId(map), [map]);
  const tree = useMemo(() => (root === undefined ? null : buildSegments(map, root)), [map, root]);
  const currentRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    currentRef.current?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [current]);

  if (!tree) return <p className="muted">No moves yet.</p>;
  const ctx = { map, size, current, onSelect, currentRef };
  return (
    <div className="movetree" role="tree" aria-label="Move tree">
      <SegmentView seg={tree} ctx={ctx} />
    </div>
  );
}

interface Ctx {
  map: NodeMap;
  size: number;
  current: number;
  onSelect: (id: number) => void;
  currentRef: React.RefObject<HTMLButtonElement | null>;
}

function SegmentView({ seg, ctx }: { seg: Segment; ctx: Ctx }) {
  return (
    <div className="seg">
      <div className="seg-run">
        {seg.ids.map((id) => {
          const n = ctx.map.get(id);
          if (!n) return null;
          const isCurrent = id === ctx.current;
          const cls = [
            "chip",
            n.color === 1 ? "chip-b" : n.color === 2 ? "chip-w" : "chip-root",
            isCurrent ? "chip-current" : "",
            n.judgment ? `chip-${n.judgment}` : "",
          ].join(" ");
          return (
            <button
              key={id}
              ref={isCurrent ? ctx.currentRef : undefined}
              className={cls}
              role="treeitem"
              aria-selected={isCurrent}
              title={[n.name, n.comment].filter(Boolean).join(" - ") || undefined}
              onClick={() => ctx.onSelect(id)}
            >
              {nodeText(n, ctx.size)}
              {n.comment || n.name ? <span className="chip-note" aria-hidden="true" /> : null}
              {hasUnloadedChildren(ctx.map, n) && (
                <span className="chip-more" aria-label="more moves not loaded">
                  +
                </span>
              )}
            </button>
          );
        })}
      </div>
      {seg.branches.map((b, i) => (
        <div key={b.ids[0]} className={i === 0 ? "seg-main" : "seg-var"}>
          <SegmentView seg={b} ctx={ctx} />
        </div>
      ))}
    </div>
  );
}
