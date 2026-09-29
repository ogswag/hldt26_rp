// Russian names of the catalog's own codes; users never see the codes.
export const kindLabels: Record<string, string> = {
  brs: 'робот',
  bas: 'беспилотник',
  software: 'ПО',
}

export const statusLabels: Record<string, string> = {
  operation: 'в эксплуатации',
  piloting: 'пилот',
  rnd: 'НИОКР',
}

// familyLabels name one robot of a catalog group; familyPluralLabels name the group in filters.
export const familyLabels: Record<string, string> = {
  mobile: 'Мобильный робот',
  uav: 'Беспилотник',
  ground: 'Наземный транспорт',
  marine: 'Морской робот',
  stationary: 'Стационарная система',
  manipulator: 'Манипулятор',
  humanoid: 'Антропоморфный робот',
  software: 'Программа',
  other: 'Другое',
}

export const familyPluralLabels: Record<string, string> = {
  mobile: 'Мобильные роботы',
  uav: 'Беспилотники',
  ground: 'Наземный транспорт',
  marine: 'Морские роботы',
  stationary: 'Стационарные системы',
  manipulator: 'Манипуляторы',
  humanoid: 'Антропоморфные роботы',
  software: 'Программы',
  other: 'Другое',
}
