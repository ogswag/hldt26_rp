import type { Solution } from '../api/client'
import { familyOf } from './families'
import { FamilyMark } from './FamilyMark'
import { robotFacts } from './values'

// RobotHoverCard is the robot's status panel: the full name over a thick rule, then ruled rows with the value at the
// right. The type carries its group mark. Everything in it is also in the robot overlay.
export function RobotHoverCard({ s }: { s: Solution }) {
  const [type, ...rest] = robotFacts(s)
  return (
    <div className="robot-card">
      <p className="robot-card-title">{s.name}</p>
      <dl className="robot-card-rows">
        <div>
          <dt>{type[0]}</dt>
          <dd className="robot-card-type">
            <FamilyMark family={familyOf(s.family)} />
            {type[1]}
          </dd>
        </div>
        {rest.map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
