/** A smooth line through a day of hourly counts, and the area under it, in a 560 by 200 frame. */
export interface SmoothChart {
  line: string
  area: string
  grid: { y: number; label: number }[]
  peak: { x: number; y: number; at: number }
  hover: { x: number; width: number; label: string }[]
}

const W = 560
const top = 10
const bottom = 176

/** The tick values for a scale: four steps up to the top. */
export function ticks(max: number): number[] {
  const step = Math.max(1, Math.ceil(max / 4))
  return [step, step * 2, step * 3, step * 4]
}

/** Draws the counts; max is the top of the scale. */
export function smoothChart(values: number[], labels: string[], max: number): SmoothChart {
  const n = values.length
  const x = (i: number) => 4 + (i * (W - 8)) / (n - 1)
  const y = (v: number) => bottom - (v / max) * (bottom - top)
  const pts = values.map((v, i) => [x(i), y(v)] as const)
  let d = `M${pts[0]?.[0].toFixed(1) ?? 0},${pts[0]?.[1].toFixed(1) ?? bottom}`
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] ?? pts[i]!
    const p1 = pts[i]!
    const p2 = pts[i + 1]!
    const p3 = pts[i + 2] ?? p2
    const c1 = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6]
    const c2 = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6]
    d += ` C${c1[0]!.toFixed(1)},${c1[1]!.toFixed(1)} ${c2[0]!.toFixed(1)},${c2[1]!.toFixed(1)} ${p2[0].toFixed(1)},${p2[1].toFixed(1)}`
  }
  const at = values.reduce((best, v, i) => (v > (values[best] ?? 0) ? i : best), 0)
  const slot = (W - 8) / (n - 1)
  return {
    line: d,
    area: `${d} L${x(n - 1).toFixed(1)},${bottom} L${x(0).toFixed(1)},${bottom} Z`,
    grid: ticks(max).map((v) => ({ y: y(v), label: v })),
    peak: { x: x(at), y: y(values[at] ?? 0), at },
    hover: values.map((v, i) => ({ x: x(i) - slot / 2, width: slot, label: `${labels[i] ?? ''} · ${String(v)}` })),
  }
}
