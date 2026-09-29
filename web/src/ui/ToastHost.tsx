import { useSyncExternalStore } from 'react'
import { useNavigate } from 'react-router-dom'

import { CloseIcon } from './icons'
import { currentNotice, hideNotice, subscribeNotice } from './notice'
import { Reflow } from './Reflow'
import { currentToast, dismissToast, subscribeToast } from './toast'

// NOTE: the countdown bar's animationend closes the toast, so hover and focus pause both the bar and the timer.
export function ToastHost() {
  const toast = useSyncExternalStore(subscribeToast, currentToast)
  const notice = useSyncExternalStore(subscribeNotice, currentNotice)
  const navigate = useNavigate()
  return (
    <div className="toast-region" role="status" aria-live="polite">
      {toast ? (
        <div className="toast" key={toast.id}>
          <p className="toast-message">
            <Reflow>{toast.message}</Reflow>
          </p>
          {toast.action ? (
            <div className="toast-actions">
              <button
                type="button"
                className="btn"
                onClick={() => {
                  dismissToast(toast.id)
                  toast.action?.run(navigate)
                }}
              >
                {toast.action.label}
              </button>
            </div>
          ) : null}
          <div
            className="toast-timer"
            aria-hidden="true"
            style={{ animationDuration: `${toast.durationMs}ms` }}
            onAnimationEnd={() => dismissToast(toast.id)}
          />
        </div>
      ) : null}
      {notice ? (
        <div className="toast notice" key={notice.id} data-busy={notice.busy ? '' : undefined}>
          <div className="notice-row">
            <p className="toast-message">
              <Reflow>{notice.message}</Reflow>
            </p>
            {notice.busy ? null : (
              <button
                type="button"
                className="icon-button"
                aria-label="Закрыть"
                title="Закрыть"
                onClick={() => hideNotice(notice.id)}
              >
                <CloseIcon size={16} />
              </button>
            )}
          </div>
          {notice.action ? (
            <div className="toast-actions">
              <button
                type="button"
                className="btn"
                onClick={() => {
                  hideNotice(notice.id)
                  notice.action?.run()
                }}
              >
                {notice.action.label}
              </button>
            </div>
          ) : null}
          {notice.busy ? <div className="notice-bar" aria-hidden="true" /> : null}
        </div>
      ) : null}
    </div>
  )
}
