import assert from 'node:assert/strict';
import { test } from 'node:test';
import { moveTab, placeTab, tabAfterClose } from '../app/utils/tabs.ts';

test('closing a tab opens the one before it, else the one after it', () => {
  assert.equal(tabAfterClose(['a', 'b', 'c'], 'b'), 'a');
  assert.equal(tabAfterClose(['a', 'b', 'c'], 'c'), 'b');
  assert.equal(tabAfterClose(['a', 'b', 'c'], 'a'), 'b');
});

test('the last tab closed leaves none, an unknown one none either', () => {
  assert.equal(tabAfterClose(['a'], 'a'), undefined);
  assert.equal(tabAfterClose([], 'a'), undefined);
  assert.equal(tabAfterClose(['a', 'b'], 'c'), undefined);
});

test('a note opened replaces the tab of the note shown, or opens one after it when asked', () => {
  const tabs = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];
  assert.deepEqual(placeTab(tabs, 'b', { id: 'x' }, false), [{ id: 'a' }, { id: 'x' }, { id: 'c' }]);
  assert.deepEqual(placeTab(tabs, 'b', { id: 'x' }, true), [{ id: 'a' }, { id: 'b' }, { id: 'x' }, { id: 'c' }]);
  assert.deepEqual(placeTab(tabs, 'z', { id: 'x' }, false), [...tabs, { id: 'x' }]);
  assert.deepEqual(placeTab([], '', { id: 'x' }, false), [{ id: 'x' }]);
});

test('a tab dropped takes the place of the one under it', () => {
  assert.deepEqual(moveTab(['a', 'b', 'c'], 0, 2), ['b', 'c', 'a']);
  assert.deepEqual(moveTab(['a', 'b', 'c'], 2, 0), ['c', 'a', 'b']);
  assert.deepEqual(moveTab(['a', 'b', 'c'], 1, 1), ['a', 'b', 'c']);
});
