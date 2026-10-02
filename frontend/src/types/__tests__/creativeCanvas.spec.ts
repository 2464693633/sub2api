import { describe, expect, it } from 'vitest'
import { validateCreativeCanvasConnection, type CreativeCanvasNode, type CreativeCanvasKind } from '../creativeCanvas'

function node(id: string, kind: CreativeCanvasKind): CreativeCanvasNode {
  return { id, type: kind, position: { x: 0, y: 0 }, data: { kind, title: id, prompt: '' } }
}

describe('creative canvas connections', () => {
  const nodes = [node('prompt', 'text'), node('overview', 'image'), node('detail', 'image'), node('clip', 'video')]
  it('allows planning image and video dependencies before a result exists', () => {
    for (const source of ['prompt', 'overview']) {
      for (const target of ['detail', 'clip']) {
        expect(validateCreativeCanvasConnection({ source, target }, nodes, [])).toBeNull()
      }
    }
  })
  it('rejects invalid endpoints, self loops, duplicate edges, and unsupported media directions', () => {
    expect(validateCreativeCanvasConnection({ source: 'missing', target: 'detail' }, nodes, [])).toBe('missing')
    expect(validateCreativeCanvasConnection({ source: 'overview', target: 'overview' }, nodes, [])).toBe('self')
    expect(validateCreativeCanvasConnection({ source: 'clip', target: 'detail' }, nodes, [])).toBe('unsupported')
    expect(validateCreativeCanvasConnection({ source: 'overview', target: 'prompt' }, nodes, [])).toBe('unsupported')
    expect(validateCreativeCanvasConnection({ source: 'overview', target: 'detail' }, nodes, [{ id: 'e', source: 'overview', target: 'detail' }])).toBe('duplicate')
  })
  it('rejects connections that create an indirect dependency cycle', () => {
    const graph = [...nodes, node('closeup', 'image')]
    const edges = [{ id: 'e1', source: 'overview', target: 'detail' }, { id: 'e2', source: 'detail', target: 'closeup' }]
    expect(validateCreativeCanvasConnection({ source: 'closeup', target: 'overview' }, graph, edges)).toBe('cycle')
    expect(validateCreativeCanvasConnection({ source: 'closeup', target: 'clip' }, graph, edges)).toBeNull()
  })
})
