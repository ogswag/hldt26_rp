type WithUAData = { userAgentData?: { platform?: string } };

// onMac is true on macOS and iPadOS, where shortcuts take Cmd instead of ctrl.
export const onMac =
  typeof navigator !== "undefined" &&
  /mac|iphone|ipad/i.test(
    (navigator as Navigator & WithUAData).userAgentData?.platform || navigator.platform,
  );

// commandKey tells whether the system's shortcut modifier is held, and only it: Cmd on macOS, ctrl elsewhere.
export function commandKey(e: KeyboardEvent): boolean {
  return onMac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
}

// shortcut writes a key combination with the system modifier.
export function shortcut(keys: string): string {
  const parts = keys
    .split("+")
    .map((key) => (key.toLowerCase() === "shift" ? (onMac ? "⇧" : "shift") : key));
  return `${onMac ? "⌘" : "ctrl"}+${parts.join("+")}`;
}

// isTyping tells whether the event went to a field, so page shortcuts leave it alone.
export function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  const tag = target.tagName;
  return (
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    target.getAttribute("role") === "combobox" ||
    target.isContentEditable
  );
}
