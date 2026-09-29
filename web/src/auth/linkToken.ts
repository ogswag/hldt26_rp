// The token of a letter travels in the link fragment, so it never reaches the server logs or a Referer header.

export function tokenFromHash(hash = window.location.hash): string {
  const raw = hash.startsWith('#') ? hash.slice(1) : hash
  const token = new URLSearchParams(raw).get('token') ?? ''
  return token.trim()
}
