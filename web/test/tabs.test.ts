import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  besideOf,
  closeIn,
  fitPanels,
  moveTab,
  openTab,
  placeTab,
  showIn,
  splitAt,
  tabAfterClose,
  withoutTab,
} from '../app/utils/tabs.ts';

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

test('a tab removed from a panel gives the note it showed to its neighbour, the last one to none', () => {
  const panel = { tabs: [{ id: 'a' }, { id: 'b' }, { id: 'c' }], active: 'b' };
  assert.deepEqual(withoutTab(panel, 'b'), { tabs: [{ id: 'a' }, { id: 'c' }], active: 'a' });
  assert.deepEqual(withoutTab(panel, 'c'), { tabs: [{ id: 'a' }, { id: 'b' }], active: 'b' });
  assert.equal(withoutTab(panel, 'z'), panel);
  assert.deepEqual(withoutTab({ tabs: [{ id: 'a' }], active: 'a' }, 'a'), { tabs: [], active: '' });
});

test('a note opened in a panel shows in its tab, else in one placed like in the first panel', () => {
  const panel = { tabs: [{ id: 'a' }, { id: 'b' }], active: 'a' };
  assert.deepEqual(openTab(panel, { id: 'b' }, true), { tabs: panel.tabs, active: 'b' });
  assert.deepEqual(openTab(panel, { id: 'x' }, false), { tabs: [{ id: 'x' }, { id: 'b' }], active: 'x' });
  assert.deepEqual(openTab(panel, { id: 'x' }, true), { tabs: [{ id: 'a' }, { id: 'x' }, { id: 'b' }], active: 'x' });
  assert.deepEqual(openTab({ tabs: [], active: '' }, { id: 'x' }, false), { tabs: [{ id: 'x' }], active: 'x' });
});

const two = () => ({
  panels: [
    { id: 'p', tabs: [{ id: 'a' }, { id: 'b' }], active: 'a' },
    { id: 'q', tabs: [{ id: 'c' }], active: 'c' },
  ],
  current: 'p',
});

test('a note shown in a panel takes its tab from the other one, which goes once emptied', () => {
  assert.deepEqual(showIn(two(), 'q', { id: 'b' }, true).panels, [
    { id: 'p', tabs: [{ id: 'a' }], active: 'a' },
    { id: 'q', tabs: [{ id: 'c' }, { id: 'b' }], active: 'b' },
  ]);
  assert.deepEqual(showIn(two(), 'p', { id: 'c' }, true), {
    panels: [{ id: 'p', tabs: [{ id: 'a' }, { id: 'c' }, { id: 'b' }], active: 'c' }],
    current: 'p',
  });
});

test('the last tab of a panel closed closes it, the current one going to its neighbour; the last panel stays', () => {
  assert.deepEqual(closeIn({ ...two(), current: 'q' }, ['c']), {
    panels: [{ id: 'p', tabs: [{ id: 'a' }, { id: 'b' }], active: 'a' }],
    current: 'p',
  });
  assert.deepEqual(closeIn(two(), ['a', 'b', 'c']), { panels: [{ id: 'p', tabs: [], active: '' }], current: 'p' });
});

test('a note dropped on an edge opens a new panel there, taken from where it was', () => {
  assert.deepEqual(splitAt(two(), { id: 'b' }, 1, 'n').panels, [
    { id: 'p', tabs: [{ id: 'a' }], active: 'a' },
    { id: 'n', tabs: [{ id: 'b' }], active: 'b' },
    { id: 'q', tabs: [{ id: 'c' }], active: 'c' },
  ]);
  assert.deepEqual(
    splitAt(two(), { id: 'c' }, 0, 'n').panels.map((panel) => panel.id),
    ['n', 'p'],
  );
});

test('a narrower screen folds the panels that no longer fit into the last one kept', () => {
  assert.deepEqual(fitPanels({ ...two(), current: 'q' }, 1), {
    panels: [{ id: 'p', tabs: [{ id: 'a' }, { id: 'b' }, { id: 'c' }], active: 'a' }],
    current: 'p',
  });
  assert.deepEqual(fitPanels(two(), 2), two());
});

test('open beside goes to the next panel, a new one with room, else the one before', () => {
  assert.deepEqual(besideOf(two(), 'p', 2), { panel: 'q' });
  assert.deepEqual(besideOf(two(), 'q', 3), { at: 2 });
  assert.deepEqual(besideOf(two(), 'q', 2), { panel: 'p' });
  assert.equal(besideOf({ panels: [two().panels[0]!], current: 'p' }, 'p', 1), undefined);
});
