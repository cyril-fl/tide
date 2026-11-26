import assert from 'node:assert/strict';
import { test } from 'node:test';
import { cycleSort, sortOf } from '../app/utils/sort.ts';

test('a click cycles a key ascending, descending, then off, keeping the others in place', () => {
  assert.equal(cycleSort('', 'title'), 'title');
  assert.equal(cycleSort('pinned,title', 'title'), 'pinned,-title');
  assert.equal(cycleSort('-title,pinned', 'title'), 'pinned');
  assert.equal(cycleSort('pinned', 'updated'), 'pinned,updated');
});

test('sortOf tells the direction of a key, false when it is not sorted on', () => {
  assert.equal(sortOf('', 'title'), false);
  assert.equal(sortOf('pinned,-created', 'created'), 'desc');
  assert.equal(sortOf('created', 'created'), 'asc');
  assert.equal(sortOf('-created', 'updated'), false);
});
