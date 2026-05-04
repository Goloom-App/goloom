import { eachDayOfInterval, endOfMonth, endOfWeek, format, isSameMonth, parseISO, startOfMonth, startOfWeek } from 'date-fns'
import type { PostRecord } from '../../types'

export function groupPostsByDay(posts: PostRecord[]) {
  const groups = new Map<string, PostRecord[]>()
  for (const post of posts) {
    const key = format(parseISO(post.scheduledAt), 'yyyy-MM-dd')
    groups.set(key, [...(groups.get(key) ?? []), post])
  }
  for (const [, list] of groups) {
    list.sort((a, b) => parseISO(a.scheduledAt).getTime() - parseISO(b.scheduledAt).getTime())
  }
  return Array.from(groups.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => ({ key, posts: value }))
}

export function groupUpcomingIntoMonths(posts: PostRecord[]) {
  const byDay = groupPostsByDay(posts)
  const out: { monthKey: string; monthLabel: string; days: { key: string; posts: PostRecord[] }[] }[] = []
  for (const g of byDay) {
    const first = parseISO(g.posts[0]!.scheduledAt)
    const monthKey = format(first, 'yyyy-MM')
    const monthLabel = format(first, 'MMMM yyyy')
    const last = out[out.length - 1]
    if (!last || last.monthKey !== monthKey) {
      out.push({ monthKey, monthLabel, days: [g] })
    } else {
      last.days.push(g)
    }
  }
  return out
}

export function calendarCellsForMonth(month: Date, posts: PostRecord[]) {
  const rangeStart = startOfWeek(startOfMonth(month), { weekStartsOn: 1 })
  const rangeEnd = endOfWeek(endOfMonth(month), { weekStartsOn: 1 })
  const days = eachDayOfInterval({ start: rangeStart, end: rangeEnd })
  const byDay = new Map<string, PostRecord[]>()
  for (const post of posts) {
    const key = format(parseISO(post.scheduledAt), 'yyyy-MM-dd')
    const list = byDay.get(key) ?? []
    list.push(post)
    byDay.set(key, list)
  }
  for (const [, list] of byDay) {
    list.sort((a, b) => parseISO(a.scheduledAt).getTime() - parseISO(b.scheduledAt).getTime())
  }
  return days.map((day) => ({
    day,
    posts: byDay.get(format(day, 'yyyy-MM-dd')) ?? [],
    inMonth: isSameMonth(day, month),
  }))
}
