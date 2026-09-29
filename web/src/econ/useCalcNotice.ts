import { useEffect, useRef } from 'react'

import { hideNotice, showNotice } from '../ui/notice'

const noticeId = 'calc'
const busyDelayMs = 400

// useCalcNotice reports a recalculation in the corner while the old numbers stay on the page: a run shows only
// once it takes longer than busyDelayMs, a failed run stays until closed or retried.
export function useCalcNotice(opts: { active: boolean; loading: boolean; error: string; retry: () => void }): void {
  const { active, loading, error } = opts
  const retry = useRef(opts.retry)
  useEffect(() => {
    retry.current = opts.retry
  })

  useEffect(() => {
    if (!active || !loading) {
      return
    }
    const timer = setTimeout(() => showNotice({ id: noticeId, busy: true, message: 'Пересчитываем...' }), busyDelayMs)
    return () => {
      clearTimeout(timer)
      hideNotice(noticeId)
    }
  }, [active, loading])

  useEffect(() => {
    if (!active || loading || !error) {
      return
    }
    showNotice({
      id: noticeId,
      busy: false,
      message: `Не удалось пересчитать. ${/[.!?]$/.test(error) ? error : `${error}.`} Цифры на странице от прошлого расчёта.`,
      action: { label: 'Повторить', run: () => retry.current() },
    })
    return () => hideNotice(noticeId)
  }, [active, loading, error])
}
