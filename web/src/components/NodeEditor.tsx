import { useEffect, useState } from "react";
import type { Judgment, TreeNode } from "../api/types";

interface Props {
  node: TreeNode;
  editable: boolean;
  onSave: (fields: { comment: string; name: string; judgment: Judgment }) => void;
}

const JUDGMENTS: { value: Judgment; label: string }[] = [
  { value: "", label: "None" },
  { value: "correct", label: "Correct" },
  { value: "playable", label: "Playable" },
  { value: "mistake", label: "Mistake" },
];

/** Shows a node's comment; owners of teaching and joseki games can edit it. */
export function NodeEditor({ node, editable, onSave }: Props) {
  const [comment, setComment] = useState(node.comment ?? "");
  const [name, setName] = useState(node.name ?? "");
  const [judgment, setJudgment] = useState<Judgment>(node.judgment ?? "");

  useEffect(() => {
    setComment(node.comment ?? "");
    setName(node.name ?? "");
    setJudgment(node.judgment ?? "");
  }, [node.id, node.comment, node.name, node.judgment]);

  if (!editable) {
    if (!node.comment && !node.name && !node.judgment) return null;
    return (
      <section className="panel-section">
        <h3>{node.name || "Comment"}</h3>
        {node.judgment && <span className={`badge badge-${node.judgment}`}>{node.judgment}</span>}
        {node.comment && <p className="comment">{node.comment}</p>}
      </section>
    );
  }

  const dirty =
    comment !== (node.comment ?? "") ||
    name !== (node.name ?? "") ||
    judgment !== (node.judgment ?? "");

  return (
    <form
      className="panel-section"
      onSubmit={(e) => {
        e.preventDefault();
        onSave({ comment, name, judgment });
      }}
    >
      <h3>Node notes</h3>
      <label className="field">
        <span>Name</span>
        <input value={name} onChange={(e) => setName(e.target.value)} maxLength={80} />
      </label>
      <label className="field">
        <span>Judgment</span>
        <select value={judgment} onChange={(e) => setJudgment(e.target.value as Judgment)}>
          {JUDGMENTS.map((j) => (
            <option key={j.value} value={j.value}>
              {j.label}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>Comment</span>
        <textarea rows={4} value={comment} onChange={(e) => setComment(e.target.value)} />
      </label>
      <button type="submit" className="btn" disabled={!dirty}>
        Save notes
      </button>
    </form>
  );
}
