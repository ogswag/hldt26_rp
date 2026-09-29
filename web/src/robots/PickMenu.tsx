import { useState } from 'react'

import type { SolutionVariant } from '../api/client'
import { WarningIcon } from '../ui/icons'
import { Popover } from '../ui/Popover'
import { Reflow } from '../ui/Reflow'

type Props = {
  solutionId: string
  name: string
  variants: SolutionVariant[]
  // warning is shown above the variants when the matching did not recommend the robot.
  warning?: string
  onPick: (variantId: string) => void
}

// Room the menu needs under its button: the warning, the heading and three variants.
const menuHeight = 160 + 96

// PickMenu asks which variant a robot goes into. A variant that already has the robot is shown but not offered.
export function PickMenu({ solutionId, name, variants, warning, onPick }: Props) {
  const [open, setOpen] = useState(false)
  const [side, setSide] = useState<'below' | 'above'>('below')
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      label={`Выбрать ${name}`}
      align="end"
      side={side}
      trigger={(t) => (
        <button
          type="button"
          className="btn"
          {...t}
          onClick={(e) => {
            // NOTE: in a table the menu stays inside the table's scroll box, so near its bottom it opens upwards.
            const box = e.currentTarget.closest('.table-wrap')?.getBoundingClientRect()
            const below = e.currentTarget.getBoundingClientRect().bottom + menuHeight
            setSide(box && below > box.bottom ? 'above' : 'below')
            t.onClick()
          }}
        >
          Выбрать
        </button>
      )}
    >
      {warning ? (
        <div className="menu-problem" role="status">
          <WarningIcon size={16} />
          <p>
            <Reflow>{warning}</Reflow>
          </p>
        </div>
      ) : null}
      <p className="menu-heading">В какой вариант</p>
      {variants.map((v) => {
        const has = v.fleet.some((f) => f.solution_id === solutionId)
        return (
          <button
            key={v.id}
            type="button"
            className="menu-item"
            disabled={has}
            onClick={() => {
              setOpen(false)
              onPick(v.id as string)
            }}
          >
            <span>{v.name}</span>
            {has ? <span className="menu-item-note">уже есть</span> : null}
          </button>
        )
      })}
    </Popover>
  )
}
