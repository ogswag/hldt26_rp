import type { CSSProperties } from 'react'

import { familyColors, type Family } from './families'

// Robot group icon in the group color. The color comes as a
// custom property, so an inverted list row can repaint the mark.
export function FamilyMark({ family, size = 16 }: { family: Family; size?: number }) {
  const Icon = family.icon
  const style = { '--family-color': familyColors(family).color } as CSSProperties
  return (
    <span className="family-mark" style={style}>
      <Icon size={size} />
    </span>
  )
}
