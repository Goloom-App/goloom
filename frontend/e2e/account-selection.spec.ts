import { test, expect } from '@playwright/test'
import {
  activeAccountIds,
  aggregateGrowthSeries,
  aggregateHeatmapBuckets,
  isAccountActive,
  isAllSelected,
  toggleAccount,
} from '../src/views/Analytics/accountSelection'

// Pure-logic coverage for the analytics account filter (issue #80). The UI cannot
// be driven end-to-end because seeding connected accounts requires live provider
// calls, so the selection/aggregation core is verified directly.

const ALL = ['a', 'b', 'c']

test.describe('account selection', () => {
  test('empty selection means all accounts are active', () => {
    expect(isAllSelected([])).toBe(true)
    for (const id of ALL) {
      expect(isAccountActive([], id)).toBe(true)
    }
    expect(activeAccountIds([], ALL)).toEqual(ALL)
  })

  test('first click from "all" filters down to one account', () => {
    const next = toggleAccount([], 'b', ALL)
    expect(next).toEqual(['b'])
    expect(isAllSelected(next)).toBe(false)
    expect(isAccountActive(next, 'b')).toBe(true)
    expect(isAccountActive(next, 'a')).toBe(false)
    expect(activeAccountIds(next, ALL)).toEqual(['b'])
  })

  test('further clicks add and remove accounts', () => {
    let sel = toggleAccount([], 'b', ALL) // ['b']
    sel = toggleAccount(sel, 'a', ALL) // ['b','a']
    expect(activeAccountIds(sel, ALL)).toEqual(['a', 'b'])
    sel = toggleAccount(sel, 'b', ALL) // ['a']
    expect(sel).toEqual(['a'])
  })

  test('removing the last account collapses back to "all"', () => {
    const sel = toggleAccount(['a'], 'a', ALL)
    expect(isAllSelected(sel)).toBe(true)
  })

  test('selecting every account collapses back to "all"', () => {
    let sel = toggleAccount([], 'a', ALL) // ['a']
    sel = toggleAccount(sel, 'b', ALL) // ['a','b']
    sel = toggleAccount(sel, 'c', ALL) // all three -> canonical "all"
    expect(isAllSelected(sel)).toBe(true)
  })
})

test.describe('analytics aggregation', () => {
  test('growth series sums followers/following/posts per date', () => {
    const merged = aggregateGrowthSeries([
      [
        { account_id: 'a', date: '2026-06-01', followers: 10, following: 2, posts: 1 },
        { account_id: 'a', date: '2026-06-02', followers: 12, following: 2, posts: 2 },
      ],
      [{ account_id: 'b', date: '2026-06-01', followers: 5, following: 1, posts: 3 }],
    ])
    expect(merged).toEqual([
      { account_id: 'a', date: '2026-06-01', followers: 15, following: 3, posts: 4 },
      { account_id: 'a', date: '2026-06-02', followers: 12, following: 2, posts: 2 },
    ])
  })

  test('heatmap buckets sum scores that share weekday/hour', () => {
    const merged = aggregateHeatmapBuckets([
      [
        { weekday: 1, hour: 10, score: 4 },
        { weekday: 2, hour: 9, score: 1 },
      ],
      [{ weekday: 1, hour: 10, score: 6 }],
    ])
    const monday10 = merged.find((b) => b.weekday === 1 && b.hour === 10)
    expect(monday10?.score).toBe(10)
    expect(merged).toHaveLength(2)
  })
})
