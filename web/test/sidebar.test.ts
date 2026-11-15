import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mergeSidebar, moveSection } from '../app/utils/sidebar.ts';

test('the saved sections keep their order, the missing ones are shown at the end, the unknown ones dropped', () => {
  assert.deepEqual(
    mergeSidebar([
      { id: 'folders', shown: false },
      { id: 'graph', shown: true },
      { id: 'recent', shown: true },
    ]).map((s) => `${s.id}:${s.shown}`),
    ['folders:false', 'recent:true', 'favorites:true', 'pinned:true', 'savedSearches:true', 'tags:true'],
  );
});

test('a moved section lands right before its target', () => {
  const sections = mergeSidebar([]);
  assert.deepEqual(
    moveSection(sections, 'folders', 'favorites').map((s) => s.id),
    ['folders', 'favorites', 'pinned', 'recent', 'savedSearches', 'tags'],
  );
  assert.deepEqual(moveSection(sections, 'x', 'favorites'), sections);
});
