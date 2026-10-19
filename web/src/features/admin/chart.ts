/** A smooth line through a day of hourly counts, and the area under it, in a 560 by 200 frame. */
export interface SmoothChart {
  line: string
  area: string
  grid: { y: number; label: number }[]
  peak: { x: number; y: number; at: number }
  hover: { x: number; width: number; label: string }[]
  xs: number[]
}

interface Point {
  x: number
  y: number
}

const W = 560
// The plot stops short of the right edge, where the scale's labels sit.
const right = W - 26
const top = 10
const bottom = 176

/** The tick values for a scale: four steps up to the top. */
export function ticks(max: number): [number, number, number, number] {
  const step = Math.max(1, Math.ceil(max / 4))
  return [step, step * 2, step * 3, step * 4]
}

/** Draws the counts; max is the top of the scale. */
export function smoothChart(values: number[], labels: string[], max: number): SmoothChart {
  const n = values.length
  const x = (i: number) => 4 + (i * (right - 4)) / (n - 1)
  const y = (v: number) => bottom - (v / max) * (bottom - top)
  const pts = values.map((v, i) => ({ x: x(i), y: y(v) }))
  // Monotone cubic (Fritsch and Carlson): smooth, but never above or below the counts it joins.
  // Each inner point takes the harmonic mean of its two slopes, or none at a turn; an end takes its one slope.
  const slopes = pts.slice(1).map((b, i) => {
    const a = pts[i] as Point
    return (b.y - a.y) / (b.x - a.x)
  })
  const inner = slopes.slice(1).map((b, i) => {
    const a = slopes[i] as number
    return a * b <= 0 ? 0 : (2 * a * b) / (a + b)
  })
  const tangents = [...slopes.slice(0, 1), ...inner, ...slopes.slice(-1)]
  const at1 = (v: number) => v.toFixed(1)
  const first = pts[0] as Point
  const d = pts.slice(1).reduce((path, b, i) => {
    const a = pts[i] as Point
    const h = (b.x - a.x) / 3
    const [ta, tb] = [tangents[i] as number, tangents[i + 1] as number]
    return `${path} C${at1(a.x + h)},${at1(a.y + h * ta)} ${at1(b.x - h)},${at1(b.y - h * tb)} ${at1(b.x)},${at1(b.y)}`
  }, `M${at1(first.x)},${at1(first.y)}`)
  const at = values.reduce((best, v, i) => (v > (values[best] as number) ? i : best), 0)
  const slot = (right - 4) / (n - 1)
  return {
    line: d,
    area: `${d} L${at1(x(n - 1))},${String(bottom)} L${at1(x(0))},${String(bottom)} Z`,
    grid: ticks(max).map((v) => ({ y: y(v), label: v })),
    peak: { x: x(at), y: y(values[at] as number), at },
    xs: values.map((_, i) => x(i)),
    hover: values.map((v, i) => ({ x: x(i) - slot / 2, width: slot, label: `${labels[i] as string} · ${String(v)}` })),
  }
}
