import { indexedDBRuns, memoryRuns } from './demoRuns'

// demoRunStore is the one store of kept demo runs, shared by the simulation page and the check on Расчёт.
export const demoRunStore = indexedDBRuns() ?? memoryRuns()
