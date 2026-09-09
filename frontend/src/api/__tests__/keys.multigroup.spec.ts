import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post, put } = vi.hoisted(() => ({
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post, put }
}))

import { create, update } from '@/api/keys'

describe('API key multi-group payloads', () => {
  beforeEach(() => {
    post.mockReset()
    put.mockReset()
    post.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
  })

  it('creates a key using only the ordered groups when group_ids are provided', async () => {
    await create('Failover key', 4, undefined, undefined, undefined, undefined, undefined, undefined, [4, 7])

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'Failover key',
      group_ids: [4, 7]
    })
  })

  it('keeps the legacy group_id payload for callers without group_ids', async () => {
    await create('Legacy key', 4)

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'Legacy key',
      group_id: 4
    })
  })

  it('updates a key using only group_ids', async () => {
    await update(12, { group_ids: [7, 4] })

    expect(put).toHaveBeenCalledWith('/keys/12', {
      group_ids: [7, 4]
    })
  })
})
