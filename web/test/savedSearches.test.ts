import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { SavedSearch } from '../app/types/items.ts';
import { nextTagState, reorderSearches, tagCriteria, toTagChips } from '../app/utils/savedSearches.ts';

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

test('a smart folder keeps a AND (b OR c) without d when opened and saved', () => {
  const saved = { tag_ids: ['a'], tag_mode: 'all', any_tag_ids: ['b', 'c'], no_tag_ids: ['d'] };
  assert.deepEqual(tagCriteria(toTagChips(saved)), saved);
});

test('tag_ids without tag_mode are one of them, the API default', () => {
  assert.deepEqual(tagCriteria(toTagChips({ tag_ids: ['a', 'b'] })), {
    tag_ids: [],
    tag_mode: 'all',
    any_tag_ids: ['a', 'b'],
    no_tag_ids: [],
  });
});

test('a click on a tag goes through must have, one of, without', () => {
  assert.deepEqual([nextTagState.all, nextTagState.any, nextTagState.not], ['any', 'not', 'all']);
});
