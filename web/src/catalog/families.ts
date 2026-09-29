import type { ComponentType } from 'react'

import {
  BoatIcon,
  CodeIcon,
  DotsThreeIcon,
  DroneIcon,
  FactoryIcon,
  HandGrabbingIcon,
  PersonSimpleIcon,
  RobotIcon,
  TruckIcon,
  type IconProps,
} from '../ui/icons'
import { familyLabels, familyPluralLabels } from './labels'

export type FamilyCode = 'mobile' | 'uav' | 'ground' | 'marine' | 'stationary' | 'manipulator' | 'humanoid' | 'software' | 'other'

export type Family = {
  code: FamilyCode
  label: string
  plural: string
  icon: ComponentType<IconProps>
  // series is the color slot (--series-N); «Другое» has none and stays gray.
  series: number | null
}

const icons: Record<FamilyCode, ComponentType<IconProps>> = {
  mobile: RobotIcon,
  uav: DroneIcon,
  ground: TruckIcon,
  marine: BoatIcon,
  stationary: FactoryIcon,
  manipulator: HandGrabbingIcon,
  humanoid: PersonSimpleIcon,
  software: CodeIcon,
  other: DotsThreeIcon,
}

export const familyCodes: FamilyCode[] = ['mobile', 'uav', 'ground', 'marine', 'stationary', 'manipulator', 'humanoid', 'software', 'other']

export const families: Family[] = familyCodes.map((code, i) => ({
  code,
  label: familyLabels[code],
  plural: familyPluralLabels[code],
  icon: icons[code],
  series: code === 'other' ? null : i + 1,
}))

export function isFamilyCode(v: unknown): v is FamilyCode {
  return typeof v === 'string' && (familyCodes as string[]).includes(v)
}

// familyOf reads a robot's group; an empty or unknown one is «Другое», as the server files it.
export function familyOf(code: string | null | undefined): Family {
  const found = families.find((f) => f.code === code)
  return found ?? (families.at(-1) as Family)
}

// familyColors are the CSS values of a group's icon and tint.
export function familyColors(f: Family): { color: string; background: string } {
  if (f.series === null) {
    return { color: 'var(--icon)', background: 'var(--bg-muted)' }
  }
  return { color: `var(--series-${f.series})`, background: `var(--series-${f.series}-bg)` }
}
