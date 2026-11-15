import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  choose,
  conflictRows,
  lineChanges,
  merge,
  mergedText,
  mergeFork,
  pickedLines,
  rebaseFork,
} from '../app/utils/merge';
import type { Choice, MergePart } from '../app/utils/merge';

// The text with every conflict resolved by the same choice.
const chosen = (parts: MergePart[], choice: Choice) =>
  mergedText(
    parts,
    parts.flatMap((part) =>
      'lines' in part ? [] : [pickedLines(conflictRows(part), choose(conflictRows(part), choice))],
    ),
  );

const base = '# Courses\n\n- Pain\n- Lait\n- Œufs';

test('changes in different places merge on their own', () => {
  const parts = merge(
    base,
    '# Courses\n\n- Pain complet\n- Lait\n- Œufs',
    '# Courses\n\n- Pain\n- Lait\n- Œufs\n- Riz',
  );
  assert.ok(parts.every((part) => 'lines' in part));
  assert.equal(mergedText(parts), '# Courses\n\n- Pain complet\n- Lait\n- Œufs\n- Riz');
});

test('the same line changed on both sides is a conflict, resolved as chosen', () => {
  const parts = merge(base, base.replace('Lait', 'Lait demi-écrémé'), base.replace('Lait', "Lait d'avoine"));
  assert.deepEqual(
    parts.filter((part) => !('lines' in part)),
    [{ mine: ['- Lait demi-écrémé'], theirs: ["- Lait d'avoine"] }],
  );
  assert.equal(chosen(parts, 'mine'), base.replace('Lait', 'Lait demi-écrémé'));
  assert.equal(chosen(parts, 'theirs'), base.replace('Lait', "Lait d'avoine"));
  assert.equal(chosen(parts, 'both'), base.replace('- Lait', "- Lait demi-écrémé\n\n- Lait d'avoine"));
});

test('the comparison gives the lines removed and added', () => {
  assert.deepEqual(lineChanges('a\nb\nc', 'a\nB\nc\nd'), [
    { kind: 'same', lines: ['a'] },
    { kind: 'removed', lines: ['b'] },
    { kind: 'added', lines: ['B'] },
    { kind: 'same', lines: ['c'] },
    { kind: 'added', lines: ['d'] },
  ]);
});

test('a fork merges back without its title: the original keeps its own', () => {
  const fork = '# Courses (version du 1 nov.)\n\n- Pain\n- Lait\n- Œufs';
  const parts = mergeFork(fork, fork.replace('- Œufs', '- Œufs\n- Riz'), base.replace('# Courses', '## Liste'));
  assert.ok(parts.every((part) => 'lines' in part));
  assert.equal(mergedText(parts), '## Liste\n\n- Pain\n- Lait\n- Œufs\n- Riz');
});

test('a fork merges into an original with no title, a heading in code being none', () => {
  const code = '```\n# not a title\n```\none';
  const fork = `# Copie\n\n${code}`;
  assert.equal(mergedText(mergeFork(fork, `${fork}\ntwo`, code)), `${code}\ntwo`);
});

test('a fork whose title was deleted keeps its next heading and the original its title (#176)', () => {
  const fork = '# Copie\n\n## Section\ntexte';
  const parts = mergeFork(fork, '## Section\ntexte\nplus', '# Courses\n\n## Section\ntexte');
  assert.ok(parts.every((part) => 'lines' in part));
  assert.equal(mergedText(parts), '# Courses\n## Section\ntexte\nplus');
});

test('a conflict of a fork shows no title line', () => {
  const fork = '# Copie\n\n- Pain';
  const parts = mergeFork(fork, '# Copie renommée\n\n- Pain complet', '# Courses\n\n- Pain blanc');
  assert.deepEqual(
    parts.filter((part) => !('lines' in part)),
    [{ mine: ['- Pain complet'], theirs: ['- Pain blanc'] }],
  );
  assert.equal(chosen(parts, 'mine'), '# Courses\n\n- Pain complet');
});

test('both sides of a fork adding a title keep it once with both', () => {
  // A base with no title (the fork untitled when last merged), a title added on both sides around changed lines.
  const parts = mergeFork('b', 'a\n# T\nc', 'z\n# T\ny');
  assert.ok(parts.some((part) => !('lines' in part)));
  assert.equal(chosen(parts, 'both'), 'a\n\nz\n# T\nc\n\ny');
});

test('a conflict resolved line by line keeps the lines picked on each side, in order', () => {
  const rows = conflictRows({ mine: ['a', 'B', 'c'], theirs: ['a', 'b', 'c', 'd'] });
  assert.deepEqual(rows, [
    { kind: 'same', line: 'a' },
    { kind: 'removed', line: 'b' },
    { kind: 'added', line: 'B' },
    { kind: 'same', line: 'c' },
    { kind: 'removed', line: 'd' },
  ]);
  assert.deepEqual(pickedLines(rows, [true, false, true, true, true]), ['a', 'B', 'c', 'd']);
  assert.deepEqual(pickedLines(rows, choose(rows, 'mine')), ['a', 'B', 'c']);
  assert.deepEqual(pickedLines(rows, choose(rows, 'theirs')), ['a', 'b', 'c', 'd']);
  assert.deepEqual(pickedLines(rows, choose(rows, 'both')), ['a', 'B', '', 'b', 'c', 'd']);
});

test('a side emptied in a conflict adds no blank line', () => {
  const rows = conflictRows({ mine: [], theirs: ['x'] });
  assert.deepEqual(pickedLines(rows, choose(rows, 'mine')), []);
});

test('a fork rebases on its original keeping its own title, a conflict showing the fork as mine', () => {
  const fork = base.replace('# Courses', '# Copie');
  const parts = rebaseFork(fork, fork.replace('- Pain', '- Pain complet'), `${base}\n- Riz`);
  assert.ok(parts.every((part) => 'lines' in part));
  assert.equal(mergedText(parts), '# Copie\n\n- Pain complet\n- Lait\n- Œufs\n- Riz');
  const conflict = rebaseFork(fork, fork.replace('Lait', 'Lait copie'), base.replace('Lait', 'Lait origine'));
  assert.deepEqual(
    conflict.filter((part) => !('lines' in part)),
    [{ mine: ['- Lait copie'], theirs: ['- Lait origine'] }],
  );
  assert.equal(chosen(conflict, 'mine'), fork.replace('Lait', 'Lait copie'));
});
