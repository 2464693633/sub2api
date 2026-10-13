export interface CanvasPoint { x: number; y: number }
export interface CanvasObstacle { id: string; x: number; y: number; width: number; height: number }

function intersects(a: CanvasPoint, b: CanvasPoint, box: CanvasObstacle): boolean {
  if (a.x === b.x) return a.x > box.x && a.x < box.x + box.width && Math.max(a.y, b.y) > box.y && Math.min(a.y, b.y) < box.y + box.height
  return a.y > box.y && a.y < box.y + box.height && Math.max(a.x, b.x) > box.x && Math.min(a.x, b.x) < box.x + box.width
}

/** Route through the spaces between cards; retain a usable path when cards overlap. */
export function routeCanvasConnection(source: CanvasPoint, target: CanvasPoint, sourceID: string, targetID: string, cards: CanvasObstacle[]) {
  const start = { x: source.x + 24, y: source.y }
  const end = { x: target.x - 24, y: target.y }
  const left = Math.min(start.x, end.x)
  const right = Math.max(start.x, end.x)
  const obstacles = cards.map(card => {
    const padding = card.id === sourceID || card.id === targetID ? 0 : 12
    return { ...card, x: card.x - padding, y: card.y - padding, width: card.width + padding * 2, height: card.height + padding * 2 }
  }).filter(card => card.x < right + 1 && card.x + card.width > left - 1)
  const candidates: CanvasPoint[][] = []
  if (target.x > source.x + 48) {
    const x = (source.x + target.x) / 2
    candidates.push([source, { x, y: source.y }, { x, y: target.y }, target])
  }
  const lanes = [...new Set([(source.y + target.y) / 2, source.y, target.y, ...obstacles.flatMap(card => [card.y - 18, card.y + card.height + 18])])]
  for (const y of lanes) candidates.push([source, start, { x: start.x, y }, { x: end.x, y }, end, target])
  let chosen = candidates[0]!
  let best = Infinity
  for (const candidate of candidates) {
    let cost = 0
    for (let i = 1; i < candidate.length; i++) {
      const a = candidate[i - 1]!, b = candidate[i]!
      cost += Math.abs(a.x - b.x) + Math.abs(a.y - b.y)
      cost += obstacles.filter(card => intersects(a, b, card)).length * 1_000_000
    }
    if (cost < best) { best = cost; chosen = candidate }
  }
  const points = chosen.filter((point, i) => i === 0 || point.x !== chosen[i - 1]!.x || point.y !== chosen[i - 1]!.y)
  let longest = -1
  let label = source
  for (let i = 1; i < points.length; i++) {
    const a = points[i - 1]!, b = points[i]!
    const length = Math.abs(a.x - b.x) + Math.abs(a.y - b.y)
    if (length > longest) { longest = length; label = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 } }
  }
  return { path: points.map((p, i) => `${i ? 'L' : 'M'} ${p.x} ${p.y}`).join(' '), label, points }
}
