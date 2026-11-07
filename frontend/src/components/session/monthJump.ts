export const MONTHS_FR = ['Janvier', 'Février', 'Mars', 'Avril', 'Mai', 'Juin', 'Juillet', 'Août', 'Septembre', 'Octobre', 'Novembre', 'Décembre']

/** Years offered by the picker, newest first, down to the oldest a watch history can plausibly reach. */
export function yearsBack(today: string, oldest = 2005): number[] {
  const y = Number(today.slice(0, 4))
  return Array.from({ length: Math.max(1, y - oldest + 1) }, (_, i) => y - i)
}

/** Month jump that never lands after the current month. */
export function clampMonth(year: number, month0: number, today: string): { y: number; m: number } {
  const ty = Number(today.slice(0, 4))
  const tm = Number(today.slice(5, 7)) - 1
  return year > ty || (year === ty && month0 > tm) ? { y: ty, m: tm } : { y: year, m: month0 }
}
