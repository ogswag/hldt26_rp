// The demo build is the app without a server: static files in a bucket, the engine in the browser and the
// project in the store. Everything it reads is baked in at build time by scripts/ci/build-demo.sh; everything
// that would write to a server is not offered.

export const demoMode = import.meta.env.VITE_APP_MODE === 'demo'

// files maps the requests a demo answers to the files it carries. A request outside this list has no answer
// here: it belongs to an account, and the demo has none.
const files: [RegExp, (m: RegExpExecArray) => string][] = [
  [/^\/api\/object-types$/, () => 'object-types.json'],
  [/^\/api\/object-types\/([a-z_]+)\/schema$/, (m) => `schema-${m[1]}.json`],
  [/^\/api\/catalog\/bundle$/, () => 'catalog.json'],
  [/^\/api\/catalog\/dictionaries$/, () => 'dictionaries.json'],
  [/^\/api\/solutions$/, () => 'solutions.json'],
  [/^\/api\/solutions\/([0-9a-f-]+)$/, (m) => `solution-${m[1]}.json`],
  // NOTE: photos keep no extension; browsers read JPEG and PNG from their first bytes.
  [/^\/api\/solutions\/([0-9a-f-]+)\/image$/, (m) => `image-${m[1]}`],
]

// demoAsset returns the file that answers this request, or null when nothing here can.
export function demoAsset(path: string): string | null {
  const clean = path.split('?')[0]
  for (const [re, name] of files) {
    const m = re.exec(clean)
    if (m) {
      return `${import.meta.env.BASE_URL}demo/${name(m)}`
    }
  }
  return null
}

export const DemoNoServer = 'Это демо работает без сервера: сохранить проект и войти в аккаунт здесь нельзя.'
