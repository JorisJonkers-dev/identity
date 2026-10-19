/** A smooth line through a day of hourly counts, and the area under it, in a 560 by 200 frame. */
export interface SmoothChart {
  line: string
  area: string
  grid: { y: number; label: number }[]
  peak: { x: number; y: number; at: number }
  hover: { x: number; width: number; label: string }[]
  xs: number[]
}

const W = 560
// The plot stops short of the right edge, where the scale's labels sit.
const right = W - 26
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
  const x = (i: number) => 4 + (i * (right - 4)) / (n - 1)
  const y = (v: number) => bottom - (v / max) * (bottom - top)
  const pts = values.map((v, i) => [x(i), y(v)] as const)
  // Monotone cubic (Fritsch and Carlson): smooth, but never above or below the counts it joins.
  const slopes = pts.slice(0, -1).map((p, i) => (pts[i + 1]![1] - p[1]) / (pts[i + 1]![0] - p[0]))
  const tangents = pts.map((_, i) => {
    if (i === 0) return slopes[0] ?? 0
    if (i === pts.length - 1) return slopes[i - 1] ?? 0
    const a = slopes[i - 1]!
    const b = slopes[i]!
    return a * b <= 0 ? 0 : (2 * a * b) / (a + b)
  })
  let d = `M${pts[0]?.[0].toFixed(1) ?? 0},${pts[0]?.[1].toFixed(1) ?? bottom}`
  for (let i = 0; i < pts.length - 1; i++) {
    const [x1, y1] = pts[i]!
    const [x2, y2] = pts[i + 1]!
    const h = (x2 - x1) / 3
    d += ` C${(x1 + h).toFixed(1)},${(y1 + h * tangents[i]!).toFixed(1)} ${(x2 - h).toFixed(1)},${(y2 - h * tangents[i + 1]!).toFixed(1)} ${x2.toFixed(1)},${y2.toFixed(1)}`
  }
  const at = values.reduce((best, v, i) => (v > (values[best] ?? 0) ? i : best), 0)
  const slot = (right - 4) / (n - 1)
  return {
    line: d,
    area: `${d} L${x(n - 1).toFixed(1)},${bottom} L${x(0).toFixed(1)},${bottom} Z`,
    grid: ticks(max).map((v) => ({ y: y(v), label: v })),
    peak: { x: x(at), y: y(values[at] ?? 0), at },
    xs: values.map((_, i) => x(i)),
    hover: values.map((v, i) => ({ x: x(i) - slot / 2, width: slot, label: `${labels[i] ?? ''} · ${String(v)}` })),
  }
}
