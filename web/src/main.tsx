import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@fontsource-variable/ibm-plex-sans'

import App from './App.tsx'
import { registerServiceWorker } from './offline/serviceWorker'
import { guardPreloadErrors } from './offline/staleChunk'
import './styles/tokens.css'
import './index.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

registerServiceWorker(new URL(import.meta.url).pathname)
guardPreloadErrors()
