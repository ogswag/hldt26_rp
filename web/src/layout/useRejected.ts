import { useEffect, useEffectEvent, useRef } from 'react'

import { lowerFirst, title } from '../store/rejected'
import { reasonText } from '../store/reasons'
import type { Rejected } from '../store/store'
import { useProjectStore, useStoreSelector } from '../store/useProjectStore'
import { showToast } from '../ui/toast'

// useRejected lists what the server did not accept. A new refusal happens while the user looks at something
// else, so it also arrives as a toast whose action opens the account menu, where the list lives.
export function useRejected(storeKey: string, onShow: () => void): readonly Rejected[] {
  const store = useProjectStore(storeKey)
  const rejected = useStoreSelector(store, () => store.getRejected())
  const seen = useRef(0)

  const announce = useEffectEvent((items: readonly Rejected[]) => {
    const last = items.at(-1) as Rejected
    showToast({
      message: `Не применено: ${lowerFirst(title(last))}. ${reasonText(last.reason)}`,
      action: { label: 'Показать', run: onShow },
    })
  })

  useEffect(() => {
    if (rejected.length > seen.current) {
      announce(rejected)
    }
    seen.current = rejected.length
  }, [rejected])

  return rejected
}
