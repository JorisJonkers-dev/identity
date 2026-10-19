import { describe, expect, it } from 'vitest'
import { smoothChart, ticks } from './chart'

const labels = ['08:00', '09:00', '10:00', '11:00', '12:00']
const points = (d: string) => [...d.matchAll(/(?:M|C[^ ]+ [^ ]+ )([\d.]+),([\d.]+)/g)].map((m) => [Number(m[1]), Number(m[2])])

describe('the scale', () => {
  it.each([
    [1, [1, 2, 3, 4]],
    [4, [1, 2, 3, 4]],
    [7, [2, 4, 6, 8]],
    [186, [47, 94, 141, 188]],
  ])('for a most of %d ticks %j', (most, want) => {
    expect(ticks(most)).toEqual(want)
  })
})

describe('the sign-ins chart', () => {
  it('passes through every count, left to right, none at the bottom and the top of the scale at the top', () => {
    const c = smoothChart([0, 2, 4, 1, 0], labels, 4)
    expect(points(c.line)).toEqual([
      [4, 176],
      [136.5, 93],
      [269, 10],
      [401.5, 134.5],
      [534, 176],
    ])
  })

  it('never rises above a peak between two counts', () => {
    const c = smoothChart([0, 4, 4, 0, 0], labels, 4)
    // A Bézier curve stays inside its control points, so none above the peak keeps the curve below it.
    const ys = [...c.line.matchAll(/,(-?[\d.]+)/g)].map((m) => Number(m[1]))
    expect(Math.min(...ys)).toBe(10)
  })

  it('stays flat where the counts do', () => {
    const c = smoothChart([0, 0, 0, 0, 0], labels, 4)
    expect([...c.line.matchAll(/,([\d.]+)/g)].every((m) => m[1] === '176.0')).toBe(true)
  })

  it('marks the first highest count as the peak', () => {
    const c = smoothChart([1, 3, 0, 3, 1], labels, 4)
    expect(c.peak).toEqual({ x: 136.5, y: 51.5, at: 1 })
  })

  it('closes the area under the line along the bottom', () => {
    const c = smoothChart([0, 1, 2, 1, 0], labels, 4)
    expect(c.area).toBe(`${c.line} L534.0,176 L4.0,176 Z`)
  })

  it('draws a grid line at each tick', () => {
    expect(smoothChart([0, 1, 2, 1, 0], labels, 4).grid).toEqual([
      { y: 134.5, label: 1 },
      { y: 93, label: 2 },
      { y: 51.5, label: 3 },
      { y: 10, label: 4 },
    ])
  })

  it('gives each hour a slot to hover, named by its hour and count', () => {
    const c = smoothChart([0, 1, 2, 1, 0], labels, 4)
    expect(c.hover[1]).toEqual({ x: 70.25, width: 132.5, label: '09:00 · 1' })
    expect(c.xs).toEqual([4, 136.5, 269, 401.5, 534])
  })
})
