import { useSyncExternalStore } from 'react'

export type ThemeChoice = 'auto' | 'light' | 'dark'
export type ResolvedTheme = 'light' | 'dark'

// NOTE: index.html reads the same key before first paint, so a saved theme never flashes.
const storageKey = 'theme'
const changeEvent = 'app-themechange'

function isChoice(v: string | null): v is ThemeChoice {
  return v === 'auto' || v === 'light' || v === 'dark'
}

export function loadThemeChoice(): ThemeChoice {
  try {
    const raw = localStorage.getItem(storageKey)
    return isChoice(raw) ? raw : 'auto'
  } catch {
    return 'auto'
  }
}

export function applyThemeChoice(choice: ThemeChoice): void {
  const root = document.documentElement
  if (choice === 'auto') {
    delete root.dataset.theme
  } else {
    root.dataset.theme = choice
  }
  try {
    if (choice === 'auto') {
      localStorage.removeItem(storageKey)
    } else {
      localStorage.setItem(storageKey, choice)
    }
  } catch {
    // NOTE: without storage the choice lasts until reload.
  }
  window.dispatchEvent(new Event(changeEvent))
}

function darkQuery(): MediaQueryList {
  return window.matchMedia('(prefers-color-scheme: dark)')
}

function currentChoice(): ThemeChoice {
  const attr = document.documentElement.dataset.theme ?? null
  return attr === 'light' || attr === 'dark' ? attr : 'auto'
}

function resolvedTheme(): ResolvedTheme {
  const choice = currentChoice()
  if (choice !== 'auto') {
    return choice
  }
  return darkQuery().matches ? 'dark' : 'light'
}

function subscribe(onChange: () => void): () => void {
  const mq = darkQuery()
  mq.addEventListener('change', onChange)
  window.addEventListener(changeEvent, onChange)
  return () => {
    mq.removeEventListener('change', onChange)
    window.removeEventListener(changeEvent, onChange)
  }
}

export function useThemeChoice(): [ThemeChoice, (choice: ThemeChoice) => void] {
  const choice = useSyncExternalStore(subscribe, currentChoice, () => 'auto' as const)
  return [choice, applyThemeChoice]
}

export function useResolvedTheme(): ResolvedTheme {
  return useSyncExternalStore(subscribe, resolvedTheme, () => 'light' as const)
}

const canvasTokens = {
  paper: '--canvas-paper',
  edge: '--canvas-edge',
  ink: '--canvas-ink',
  inkMuted: '--canvas-ink-muted',
  line: '--canvas-line',
  lineFaint: '--canvas-line-faint',
  zone: '--canvas-zone',
  zoneAlt: '--canvas-zone-alt',
  zoneStrong: '--canvas-zone-strong',
  zoneStorage: '--canvas-zone-storage',
  zoneDock: '--canvas-zone-dock',
  zonePick: '--canvas-zone-pick',
  zoneCharge: '--canvas-zone-charge',
  obstacle: '--canvas-obstacle',
  obstacleEdge: '--canvas-obstacle-edge',
  issue: '--canvas-issue',
  halo: '--canvas-halo',
  select: '--canvas-select',
  danger: '--canvas-danger',
  warning: '--canvas-warning',
  path: '--canvas-path',
  pathFaint: '--canvas-path-faint',
  task: '--canvas-task',
  dock: '--canvas-dock',
  charger: '--canvas-charger',
  gate: '--canvas-gate',
  other: '--canvas-other',
  segment: '--canvas-segment',
  drop: '--canvas-drop',
  idle: '--canvas-idle',
  cargo: '--canvas-cargo',
  cursor: '--canvas-cursor',
  presence1: '--presence-1',
  presence2: '--presence-2',
  presence3: '--presence-3',
  presence4: '--presence-4',
} as const

// presenceColor picks a person's colour out of the palette.
export function presenceColor(pal: CanvasPalette, n: number): string {
  return [pal.presence1, pal.presence2, pal.presence3, pal.presence4][n % 4]
}

// font is the family canvas labels are drawn in: Plex Sans once it has loaded, the system face before that. A
// canvas measures text when it draws, so the switch makes Konva measure again with the real face.
export type CanvasPalette = Record<keyof typeof canvasTokens, string> & { font: string }

const sansFace = '12px "IBM Plex Sans Variable"'
const systemFont = 'ui-sans-serif, system-ui, sans-serif'

function sansReady(): boolean {
  const fonts = typeof document !== 'undefined' ? document.fonts : undefined
  return !fonts || fonts.check(sansFace)
}

export function readCanvasPalette(): CanvasPalette {
  const style = getComputedStyle(document.documentElement)
  const out = {} as CanvasPalette
  for (const [key, token] of Object.entries(canvasTokens) as [keyof typeof canvasTokens, string][]) {
    out[key] = style.getPropertyValue(token).trim()
  }
  out.font = sansReady() ? style.getPropertyValue('--font-sans').trim().replace(/\s+/g, ' ') || systemFont : systemFont
  return out
}

let paletteCache: { key: string; palette: CanvasPalette } | null = null

// NOTE: the stylesheet switches before the theme store notifies, so a read keyed by theme is current.
function paletteSnapshot(): CanvasPalette {
  const key = `${resolvedTheme()}:${sansReady()}`
  if (paletteCache?.key !== key) {
    paletteCache = { key, palette: readCanvasPalette() }
  }
  return paletteCache.palette
}

function subscribePalette(onChange: () => void): () => void {
  const off = subscribe(onChange)
  const fonts = document.fonts
  if (!fonts) {
    return off
  }
  let live = true
  fonts.addEventListener('loadingdone', onChange)
  if (!sansReady()) {
    void fonts.load(sansFace).then(() => {
      if (live) {
        onChange()
      }
    })
  }
  return () => {
    live = false
    off()
    fonts.removeEventListener('loadingdone', onChange)
  }
}

export function useCanvasPalette(): CanvasPalette {
  return useSyncExternalStore(subscribePalette, paletteSnapshot)
}
