import { createHash } from 'node:crypto'
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vitest/config'

const code = /\.(?:js|mjs)$/
const shellFile = /\.(?:css|woff2?)$/
// NOTE: Plex Serif is defined as --font-serif but no page uses it yet, so its files stay out of the precache.
const unusedFont = /ibm-plex-serif/
const demoBuild = process.env.VITE_APP_MODE === 'demo'

// serviceWorker writes sw.js from sw-template.js, filling in this build's files and a version derived from
// them. A build that changes nothing keeps its version, so the app does not ask people to reload for nothing.
//
// The shell is what the app cannot open without: the page, the entry code, the styles and the fonts. Install
// waits for it. The rest of the chunks (the map, the simulation, the export formats) are fetched behind the
// install, so a slow first visit is not held up by megabytes it may never need.
// publicFiles lists what a demo build carries beside the bundle: the browser engine and the reference data
// that a running installation would serve from its API. They are not part of the bundle, so the worker would
// not know about them; without them the demo cannot calculate offline.
function publicFiles(dir: string): { path: string; hash: string }[] {
  const at = fileURLToPath(new URL(`./public/${dir}`, import.meta.url))
  if (!existsSync(at)) {
    return []
  }
  return readdirSync(at).map((name) => ({
    path: `/${dir}/${name}`,
    // Their names carry no hash, so the build version has to take their contents into account: otherwise a
    // deployment with a new catalog would look like the same build and nobody would be offered the update.
    hash: createHash('sha256').update(readFileSync(`${at}/${name}`)).digest('hex').slice(0, 12),
  }))
}

function serviceWorker(): Plugin {
  return {
    name: 'robots-service-worker',
    apply: 'build',
    generateBundle(_options, bundle) {
      if (!demoBuild) {
        this.emitFile({
          type: 'asset',
          fileName: 'favicon.svg',
          source: readFileSync(fileURLToPath(new URL('./public/favicon.svg', import.meta.url))),
        })
      }
      const shell: string[] = ['/index.html', '/favicon.svg']
      const rest: string[] = []
      for (const [name, item] of Object.entries(bundle)) {
        const entry = item.type === 'chunk' && item.isEntry
        if (unusedFont.test(name)) {
          continue
        }
        if (shellFile.test(name) || entry) {
          shell.push(`/${name}`)
        } else if (code.test(name)) {
          rest.push(`/${name}`)
        }
      }
      const carried = demoBuild ? [...publicFiles('engine'), ...publicFiles('demo')] : []
      rest.push(...carried.map((f) => f.path))
      shell.sort()
      rest.sort()
      const stamp = carried.map((f) => `${f.path}:${f.hash}`).sort()
      const version = createHash('sha256').update([...shell, ...rest, ...stamp].join('\n')).digest('hex').slice(0, 12)
      const template = readFileSync(fileURLToPath(new URL('./sw-template.js', import.meta.url)), 'utf8')
      this.emitFile({
        type: 'asset',
        fileName: 'sw.js',
        source: template
          .replace('__VERSION__', version)
          .replace('__PRECACHE__', JSON.stringify(shell, null, 2))
          .replace('__PREFETCH__', JSON.stringify(rest, null, 2)),
      })
    },
  }
}

export default defineConfig({
  publicDir: demoBuild ? 'public' : false,
  plugins: [react(), serviceWorker()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
      '/version': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
