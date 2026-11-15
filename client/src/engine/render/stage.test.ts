import { expect, test } from 'vitest'
import { centre } from './stage'

test('a view follows its point, stops at the edge of what it may show, and centres on a span narrower than it', () => {
  // A view 10 wide over 0 to 100.
  expect(centre(40, 5, 0, 100)).toBe(40)
  expect(centre(2, 5, 0, 100)).toBe(5)
  expect(centre(99, 5, 0, 100)).toBe(95)
  // Over 0 to 8, or exactly 0 to 10, it can't move.
  expect(centre(2, 5, 0, 8)).toBe(4)
  expect(centre(9, 5, 0, 10)).toBe(5)
})
