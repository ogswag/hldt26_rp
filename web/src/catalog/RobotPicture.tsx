import { useState } from 'react'

import { solutionImagePath, type Solution } from '../api/client'
import { FileDrop } from '../ui/FileDrop'
import { familyColors, familyOf } from './families'

export const photoTypes = 'image/jpeg,image/png,image/webp,image/gif'

type Edit = {
  busy: boolean
  onPut: (file: File) => void
  onDelete: () => void
}

// RobotPicture is the robot's photo, or its group's icon on the group's tint with «Нет фото» (P1). With edit it
// takes a dropped or picked photo.
export function RobotPicture({ s, edit }: { s: Solution; edit?: Edit }) {
  const src = solutionImagePath(s)
  const [broken, setBroken] = useState<string | null>(null)
  const shown = src && broken !== src ? src : null
  const family = familyOf(s.family)
  const colors = familyColors(family)
  const Icon = family.icon

  const picture = shown ? (
    <img className="robot-photo" src={shown} alt={`Фото: ${s.name}`} onError={() => setBroken(shown)} />
  ) : (
    <div className="robot-photo is-empty" style={{ background: colors.background }}>
      <span className="robot-photo-icon" style={{ color: colors.color }}>
        <Icon size={56} />
      </span>
      <span className="robot-photo-note">Нет фото</span>
    </div>
  )

  if (!edit) {
    return <div className="robot-picture">{picture}</div>
  }
  return (
    <FileDrop accept={photoTypes} label="Фото робота" onFile={edit.onPut} disabled={edit.busy} className="robot-picture">
      {(choose) => (
        <>
          {picture}
          <div className="robot-picture-actions">
            <button type="button" className="btn" disabled={edit.busy} onClick={choose}>
              Загрузить фото
            </button>
            {s.image_sha ? (
              <button type="button" className="btn btn-danger" disabled={edit.busy} onClick={edit.onDelete}>
                Удалить фото
              </button>
            ) : null}
          </div>
        </>
      )}
    </FileDrop>
  )
}
