import { useEffect } from "react";

import { commandKey, isTyping, onMac, shortcut } from "../ui/keys";
import { showToast } from "../ui/toast";

import { reasonText } from "./reasons";
import type { UndoHistory } from "./undo";
import { useUndoHistory } from "./useProjectStore";

export const undoKeys = shortcut("Z");
export const redoKeys = shortcut("Shift+Z");

// undoStep undoes or redoes one step of this tab. Undoing what someone else already removed is refused, and the
// toast says why.
export function undoStep(history: UndoHistory, redo: boolean): void {
  const out = redo ? history.redo() : history.undo();
  if (out?.status === "rejected") {
    showToast({
      message: `${redo ? "Повтор не применён" : "Отмена не применена"}. ${reasonText(out.reason)}`,
    });
  }
}

// useUndoShortcuts gives every page of a project undo and redo over this tab's own actions: Cmd+Z and
// Cmd+Shift+Z on macOS, ctrl+Z, ctrl+Shift+Z and ctrl+Y elsewhere. Undoing what someone else already removed is
// refused, and the toast says why.
export function useUndoShortcuts(projectId: string): void {
  const [history] = useUndoHistory(projectId);
  useEffect(() => {
    if (!projectId) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      const key = e.key.toLowerCase();
      const redoY = key === "y" && !onMac;
      if (!commandKey(e) || (key !== "z" && !redoY) || isTyping(e.target)) {
        return;
      }
      e.preventDefault();
      undoStep(history, redoY || e.shiftKey);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [projectId, history]);
}
