// Shown wherever the browser computed the numbers instead of the server. The same engine code produced them,
// but nothing was saved: a run in the history always comes from the server. A demo build has no server at
// all, so there is nothing to be preliminary against and the note stays away; the sidebar says so once.

import { demoMode } from '../api/demo'
import { Reflow } from '../ui/Reflow'

type Props = {
  preliminary: boolean | undefined
  what?: string
}

export function PreliminaryNote({ preliminary, what = 'Расчёт' }: Props) {
  if (!preliminary || demoMode) {
    return null
  }
  return (
    <p className="preliminary-banner">
      <Reflow>
        <span className="tag tag-info">Предварительно</span> {what} выполнен в браузере по сохранённому каталогу,
        потому что сервер сейчас недоступен. Результат не сохранён. Повторите, когда связь вернётся.
      </Reflow>
    </p>
  )
}
