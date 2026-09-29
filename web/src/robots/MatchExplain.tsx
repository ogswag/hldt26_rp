import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import type { MatchItem, ScoreParts } from '../api/client'
import { fieldByCode, useCatalogFields } from '../catalog/fields'
import { Rows } from '../catalog/RobotView'
import { sourceHost } from '../catalog/values'
import { isObjectType } from '../guest/store'
import { objectSchema } from '../offline/reference'
import { Reflow } from '../ui/Reflow'

import { partRows, ruleLabel, stepsOf, stepValue, stepVerdict } from './explain'

type Props = {
  item: MatchItem
  weights: ScoreParts
  objectType: string
}

// MatchExplain shows how a robot's verdict and score came about: the score by part with its weights, then every
// check with the object's value, the robot's value, the result and where the robot's value comes from.
export function MatchExplain({ item, weights, objectType }: Props) {
  const catalogFields = useCatalogFields()
  const schemaQ = useQuery({
    queryKey: ['schema', objectType],
    queryFn: () => objectSchema(objectType as Parameters<typeof objectSchema>[0]),
    enabled: isObjectType(objectType),
  })
  const objectLabels = useMemo(
    () => new Map((schemaQ.data?.groups ?? []).flatMap((g) => g.fields).map((f) => [f.id, f.short || f.label])),
    [schemaQ.data],
  )
  const steps = stepsOf(item)
  return (
    <div className="match-explain">
      <Rows rows={partRows(item, weights)} />
      {steps.length > 0 ? (
        <div className="table-wrap">
          <table className="match-steps">
            <thead>
              <tr>
                <th>Проверка</th>
                <th>Объект</th>
                <th>Робот</th>
                <th>Итог</th>
                <th>Источник</th>
              </tr>
            </thead>
            <tbody>
              {steps.map((st, i) => {
                const verdict = stepVerdict(st)
                const objectValue = stepValue(st.object_value, st.object_unit)
                const solutionValue = stepValue(st.solution_value, st.solution_unit)
                return (
                  <tr key={`${st.rule_id}:${st.task_code ?? ''}:${st.solution_field ?? ''}:${i}`}>
                    <td>
                      {ruleLabel(st.rule_id)}
                      <div className="cell-note">
                        <Reflow>{st.text}</Reflow>
                      </div>
                    </td>
                    <td>
                      {objectValue ? (
                        <>
                          {objectValue}
                          <div className="cell-note">{st.object_field ? (objectLabels.get(st.object_field) ?? '') : ''}</div>
                        </>
                      ) : null}
                    </td>
                    <td>
                      {solutionValue ? (
                        <>
                          {solutionValue}
                          <div className="cell-note">{st.solution_field ? (fieldByCode(catalogFields, st.solution_field)?.label ?? '') : ''}</div>
                        </>
                      ) : null}
                    </td>
                    <td>
                      <span className={`tag ${verdict.tag}`}>{verdict.label}</span>
                    </td>
                    <td>
                      {st.source_url ? (
                        <a href={st.source_url} target="_blank" rel="noopener noreferrer">
                          {sourceHost(st.source_url)}
                        </a>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  )
}
