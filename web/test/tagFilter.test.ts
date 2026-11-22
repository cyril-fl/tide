import assert from 'node:assert/strict';
import { test } from 'node:test';
import { nextTagFilter, readTagFilter, tagFilterPath } from '../app/utils/tagFilter.ts';

test('readTagFilter reads the path and the query, nothing outside the tags', () => {
  assert.deepEqual(readTagFilter({ path: '/tags/a', params: { value: 'a' }, query: { and: 'b,c', not: 'd' } }), {
    and: ['a', 'b', 'c'],
    not: ['d'],
  });
  assert.deepEqual(readTagFilter({ path: '/tags', params: { value: '' }, query: { not: 'd' } }), {
    and: [],
    not: ['d'],
  });
  assert.deepEqual(readTagFilter({ path: '/notes/1', params: { id: '1' }, query: { not: 'd' } }), {
    and: [],
    not: [],
  });
});

test('a click cycles a tag through included, excluded, off', () => {
  let filter = { and: ['a'], not: [] as string[] };
  filter = nextTagFilter(filter, 'b');
  assert.deepEqual(filter, { and: ['a', 'b'], not: [] });
  filter = nextTagFilter(filter, 'b');
  assert.deepEqual(filter, { and: ['a'], not: ['b'] });
  filter = nextTagFilter(filter, 'b');
  assert.deepEqual(filter, { and: ['a'], not: [] });
});

test('tagFilterPath puts the first included tag in the path, an empty filter goes home', () => {
  assert.deepEqual(tagFilterPath({ and: ['a', 'b'], not: ['c'] }), {
    path: '/tags/a',
    query: { and: 'b', not: 'c' },
  });
  assert.deepEqual(tagFilterPath({ and: ['été'], not: [] }), { path: '/tags/%C3%A9t%C3%A9', query: {} });
  assert.deepEqual(tagFilterPath({ and: [], not: ['c'] }), { path: '/tags', query: { not: 'c' } });
  assert.equal(tagFilterPath({ and: [], not: [] }), '/');
});
