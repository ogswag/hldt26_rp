import { afterEach, describe, expect, it, vi } from "vitest";

async function keysOn(platform: string) {
  vi.resetModules();
  vi.stubGlobal("navigator", { platform });
  return import("./keys");
}

const press = (mods: { ctrlKey?: boolean; metaKey?: boolean }) =>
  ({ ctrlKey: false, metaKey: false, ...mods }) as KeyboardEvent;

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("shortcuts", () => {
  it("take Cmd on macOS and leave ctrl alone", async () => {
    const keys = await keysOn("MacIntel");
    expect(keys.shortcut("shift+Z")).toBe("⌘+⇧+Z");
    expect(keys.commandKey(press({ metaKey: true }))).toBe(true);
    expect(keys.commandKey(press({ ctrlKey: true }))).toBe(false);
  });

  it("take ctrl elsewhere and leave the Windows key alone", async () => {
    const keys = await keysOn("Win32");
    expect(keys.shortcut("Shift+Z")).toBe("ctrl+shift+Z");
    expect(keys.commandKey(press({ ctrlKey: true }))).toBe(true);
    expect(keys.commandKey(press({ metaKey: true }))).toBe(false);
  });
});
