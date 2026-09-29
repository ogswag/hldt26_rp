import { Children, Fragment, isValidElement, type ReactNode } from 'react'

import { layout, type Piece } from './breaks'
import { AbbreviationText } from './Abbreviation'

const NBSP = '\u00a0'

function draw(p: Piece, key: number): ReactNode {
  if (p.kind === 'word') {
    return p.slash ? (
      <span key={key} className="reflow-unit">
        <AbbreviationText text={p.text} />
      </span>
    ) : (
      <AbbreviationText key={key} text={p.text} />
    )
  }
  if (p.kind === 'join') {
    return <Fragment key={key}>{between(p.parts, NBSP)}</Fragment>
  }
  return (
    <span key={key} className="reflow-keep">
      {between(p.parts, ' ')}
    </span>
  )
}

function between(parts: Piece[], gap: string): ReactNode[] {
  return parts.flatMap((p, i) => (i === 0 ? [draw(p, i)] : [gap, draw(p, i)]))
}

function drawText(text: string, key: number): ReactNode {
  const lead = /^ */.exec(text)?.[0] ?? ''
  const trail = / *$/.exec(text)?.[0] ?? ''
  return (
    <Fragment key={key}>
      {lead}
      {between(layout(text), ' ')}
      {trail}
    </Fragment>
  )
}

// Reflow draws running text by the reflow rule (ui/breaks.ts). Text and numbers among its children are read as
// one text; elements such as tags pass through. Use it for text that wraps, not for a one-line label cut with an
// ellipsis: a kept run is an inline block, and the ellipsis cannot cut into it.
export function Reflow({ children }: { children: ReactNode }) {
  const out: ReactNode[] = []
  let text = ''
  const flush = () => {
    if (text) {
      out.push(drawText(text, out.length))
      text = ''
    }
  }
  for (const child of Children.toArray(children)) {
    if (typeof child === 'string' || typeof child === 'number') {
      text += String(child)
    } else if (isValidElement(child)) {
      flush()
      out.push(child)
    }
  }
  flush()
  return <>{out}</>
}
