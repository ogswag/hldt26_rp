import { usePresence } from '../store/useProjectStore'

const MAX = 3

// PresenceFaces shows who else has this project open. Hovering a circle names the person.
export function PresenceFaces({ projectId }: { projectId: string }) {
  const people = usePresence(projectId)
  if (people.length === 0) {
    return null
  }
  const shown = people.slice(0, MAX)
  const rest = people.length - shown.length
  return (
    <span className="faces" aria-label={`В проекте: ${people.map((p) => p.email).join(', ')}`}>
      {shown.map((p) => (
        <span key={p.userId} className="face" style={{ background: `var(--presence-${p.color + 1})` }} title={p.email}>
          {p.initials}
        </span>
      ))}
      {rest > 0 ? (
        <span className="face is-rest" title={people.slice(MAX).map((p) => p.email).join(', ')}>
          +{rest}
        </span>
      ) : null}
    </span>
  )
}
