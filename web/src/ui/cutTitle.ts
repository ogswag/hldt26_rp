import type { MouseEvent } from 'react'

// cutTitle gives an element cut by CSS (an ellipsis or a line clamp) its full text as a hover title, and no title
// while it shows whole.
export function cutTitle(full: string) {
  return (e: MouseEvent<HTMLElement>) => {
    const el = e.currentTarget
    const cut = el.scrollWidth > el.clientWidth || el.scrollHeight > el.clientHeight
    el.title = cut ? full : ''
  }
}
