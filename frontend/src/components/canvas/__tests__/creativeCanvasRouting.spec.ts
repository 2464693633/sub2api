import { describe, expect, it } from 'vitest'
import { routeCanvasConnection, type CanvasPoint, type CanvasObstacle } from '../creativeCanvasRouting'
function crosses(a: CanvasPoint, b: CanvasPoint, box: CanvasObstacle) {
  return a.x === b.x
    ? a.x > box.x && a.x < box.x + box.width && Math.max(a.y, b.y) > box.y && Math.min(a.y, b.y) < box.y + box.height
    : a.y > box.y && a.y < box.y + box.height && Math.max(a.x, b.x) > box.x && Math.min(a.x, b.x) < box.x + box.width
}
describe('canvas connection routing', () => {
  it('routes a reference past another card without implying a connection through it', () => {
    const blocker = { id: 'detail', x: 430, y: 50, width: 286, height: 330 }
    const route = routeCanvasConnection({ x: 336, y: 650 }, { x: 800, y: 210 }, 'overview', 'material', [
      { id: 'overview', x: 50, y: 480, width: 286, height: 330 }, blocker,
      { id: 'material', x: 800, y: 50, width: 286, height: 330 },
      { id: 'structure', x: 430, y: 480, width: 286, height: 330 }
    ])
    for (let i = 1; i < route.points.length; i++) expect(crosses(route.points[i - 1]!, route.points[i]!, blocker)).toBe(false)
    expect(route.points[0]).toEqual({ x: 336, y: 650 })
    expect(route.points.at(-1)).toEqual({ x: 800, y: 210 })
  })
  it('can connect to a card to the left without cutting through either card', () => {
    const cards = [{ id: 'a', x: 500, y: 50, width: 286, height: 330 }, { id: 'b', x: 50, y: 50, width: 286, height: 330 }]
    const route = routeCanvasConnection({ x: 786, y: 200 }, { x: 50, y: 200 }, 'a', 'b', cards)
    for (let i = 1; i < route.points.length; i++) for (const card of cards) expect(crosses(route.points[i - 1]!, route.points[i]!, card)).toBe(false)
  })
})
