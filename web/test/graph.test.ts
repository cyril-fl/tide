import assert from 'node:assert/strict';
import { test } from 'node:test';
import { linkTargets } from '../app/utils/graph.ts';

test('the [[ menu offers the other named notes, the closest ones first for a Pro account', () => {
  const notes = [
    { id: 'a', title: 'A', path: '/A' },
    { id: 'b', title: 'B', path: '/B' },
    { id: 'c', title: 'C', path: '/C' },
    { id: 'd', title: 'D', path: '/D' },
    { id: 'e', title: '', path: '/' },
  ];
  const neighbors = [
    { id: 'c', title: 'C', path: '/C', shared: 2 },
    { id: 'b', title: 'B', path: '/B', shared: 1 },
  ];
  const titles = (id: string | undefined, near?: typeof neighbors) =>
    linkTargets(notes, id, near).map((note) => note.title);
  assert.deepEqual(titles('a', neighbors), ['C', 'B', 'D']);
  assert.deepEqual(titles('a'), ['B', 'C', 'D']);
  assert.deepEqual(titles(undefined), ['A', 'B', 'C', 'D']);
  assert.deepEqual(
    linkTargets([], 'a', neighbors).map((note) => note.title),
    ['C', 'B'],
  );
});

test('the neighbors the API left out join the [[ menu when they hold the text typed (#150)', () => {
  const found = [{ id: 'x', title: 'Projet', path: '/Projet' }];
  const neighbors = [
    { id: 'y', title: 'Courses', path: '/Projets/Courses', shared: 1 },
    { id: 'z', title: 'Lait', path: '/Lait', shared: 3 },
  ];
  assert.deepEqual(
    linkTargets(found, undefined, neighbors, 'PROJET').map((note) => note.id),
    ['y', 'x'],
  );
});
