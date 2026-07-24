import { beforeEach, describe, expect, it } from 'vitest'
import {
  PERSONAL_GALLERY_MAX_RECORDS,
  PERSONAL_GALLERY_TTL_MS,
  __resetPersonalGalleryMemoryForTests,
  selectPersonalGalleryOverflowIDs,
} from '../personalGalleryStore'

describe('selectPersonalGalleryOverflowIDs', () => {
  beforeEach(() => {
    __resetPersonalGalleryMemoryForTests()
  })

  it('removes expired records first', () => {
    const now = 1_000_000
    const ids = selectPersonalGalleryOverflowIDs([
      { id: 'old', createdAt: now - PERSONAL_GALLERY_TTL_MS - 1, expiresAt: now - 1 },
      { id: 'fresh', createdAt: now - 10, expiresAt: now + PERSONAL_GALLERY_TTL_MS },
    ], now, 500)
    expect(ids).toEqual(['old'])
  })

  it('trims oldest alive records when over the count cap', () => {
    const now = Date.now()
    const records = Array.from({ length: 5 }, (_, index) => ({
      id: `r${index}`,
      createdAt: now - index * 1000,
      expiresAt: now + PERSONAL_GALLERY_TTL_MS,
    }))
    const ids = selectPersonalGalleryOverflowIDs(records, now, 3)
    expect(ids).toEqual(['r3', 'r4'])
  })

  it('uses the default 500-record cap constant', () => {
    expect(PERSONAL_GALLERY_MAX_RECORDS).toBe(500)
  })
})
