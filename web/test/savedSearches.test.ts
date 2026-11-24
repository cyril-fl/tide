import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { SavedSearch } from '../app/types/items.ts';
import { reorderSearches } from '../app/utils/savedSearches.ts';

const search = (id: string, extra: Partial<SavedSearch> = {}) => ({ id, ...extra }) as SavedSearch;
const ids = (searches: SavedSearch[]) => searches.map((s) => `${s.id}${s.parent_id ? `<${s.parent_id}` : ''}`);

test('a smart folder dropped on a group goes inside it, at the end', () => {
  const searches = [search('a'), search('g', { group: true }), search('b', { parent_id: 'g' })];
  assert.deepEqual(ids(reorderSearches(searches, 'a', 'g')), ['g', 'b<g', 'a<g']);
});

test('a smart folder dropped on another lands right before it, in its group', () => {
  const searches = [search('a'), search('g', { group: true }), search('b', { parent_id: 'g' }), search('c')];
  assert.deepEqual(ids(reorderSearches(searches, 'c', 'b')), ['a', 'g', 'c<g', 'b<g']);
  assert.deepEqual(ids(reorderSearches(searches, 'a', 'c')), ['g', 'b<g', 'a', 'c']);
});

test('a group dropped on a group lands before it; a group never goes inside itself', () => {
  const searches = [search('g', { group: true }), search('h', { group: true }), search('b', { parent_id: 'g' })];
  assert.deepEqual(ids(reorderSearches(searches, 'h', 'g')), ['h', 'g', 'b<g']);
  assert.equal(reorderSearches(searches, 'g', 'b'), searches);
  assert.equal(reorderSearches(searches, 'x', 'g'), searches);
});
