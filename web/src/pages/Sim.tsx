import { useCalcResult } from '../econ/useCalcResult'
import { DemoSim } from '../sim/DemoSim'
import { ProjectSim } from '../sim/ProjectSim'

export function Sim() {
  const calc = useCalcResult()
  if (calc.projectId) {
    return <ProjectSim projectId={calc.projectId} />
  }
  return <DemoSim calc={calc} />
}
