const hitMs = 2000
const waitMs = 3000
const controls = 'input:not([type="hidden"]), textarea, [role="combobox"], button:not(.field-help-button), a[href]'

function find(searchId: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-search-id="${CSS.escape(searchId)}"]`)
}

// waitFor resolves with the element once it is in the page, or null after waitMs: the target of a jump may
// render only after the route, the project or a dialog loads.
export function waitFor(searchId: string, signal?: AbortSignal): Promise<HTMLElement | null> {
  const now = find(searchId)
  if (now) {
    return Promise.resolve(now)
  }
  return new Promise((resolve) => {
    const finish = (el: HTMLElement | null) => {
      clearTimeout(timer)
      mo.disconnect()
      signal?.removeEventListener('abort', abort)
      resolve(el)
    }
    const abort = () => finish(null)
    const mo = new MutationObserver(() => {
      const el = find(searchId)
      if (el) {
        finish(el)
      }
    })
    mo.observe(document.body, { childList: true, subtree: true })
    const timer = setTimeout(() => finish(null), waitMs)
    signal?.addEventListener('abort', abort)
  })
}

export function nextFrame(): Promise<void> {
  return new Promise((r) => requestAnimationFrame(() => r()))
}

function reducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

// flash restarts the highlight on el, so a second jump to the same row shows again.
export function flash(el: HTMLElement): void {
  el.classList.remove('search-hit')
  void el.offsetWidth
  el.classList.add('search-hit')
  const prev = Number(el.dataset.hitTimer)
  if (prev) {
    clearTimeout(prev)
  }
  el.dataset.hitTimer = String(
    window.setTimeout(() => {
      el.classList.remove('search-hit')
      delete el.dataset.hitTimer
    }, hitMs),
  )
}

export function focusIn(el: HTMLElement): void {
  const target = el.matches(controls) ? el : el.querySelector<HTMLElement>(controls)
  if (target && !target.matches(':disabled')) {
    target.focus({ preventScroll: true })
    return
  }
  if (!el.hasAttribute('tabindex')) {
    el.tabIndex = -1
  }
  el.focus({ preventScroll: true })
}

// reveal scrolls the element marked data-search-id into the middle of the window and highlights it; with focus
// it also puts focus on its first control. It resolves false when the element never appeared.
export async function reveal(searchId: string, opts: { focus?: boolean; signal?: AbortSignal } = {}): Promise<boolean> {
  // NOTE: a frame before looking lets a jump to another page render it, so the old page's copy is not taken; a
  // frame after lets the page finish what it started with the element, such as opening its dialog.
  await nextFrame()
  const el = await waitFor(searchId, opts.signal)
  await nextFrame()
  if (!el || !el.isConnected || opts.signal?.aborted) {
    return false
  }
  el.scrollIntoView({ block: 'center', behavior: reducedMotion() ? 'auto' : 'smooth' })
  flash(el)
  if (opts.focus) {
    focusIn(el)
  }
  return true
}
