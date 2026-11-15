import assert from 'node:assert/strict';
import { test } from 'node:test';
import { lineChanges, merge, mergedText, mergeFork } from '../app/utils/merge';

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
  assert.equal(mergedText(parts, ['mine']), base.replace('Lait', 'Lait demi-écrémé'));
  assert.equal(mergedText(parts, ['theirs']), base.replace('Lait', "Lait d'avoine"));
  assert.equal(mergedText(parts, ['both']), base.replace('- Lait', "- Lait demi-écrémé\n\n- Lait d'avoine"));
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

test('a conflict of a fork shows no title line', () => {
  const fork = '# Copie\n\n- Pain';
  const parts = mergeFork(fork, '# Copie renommée\n\n- Pain complet', '# Courses\n\n- Pain blanc');
  assert.deepEqual(
    parts.filter((part) => !('lines' in part)),
    [{ mine: ['- Pain complet'], theirs: ['- Pain blanc'] }],
  );
  assert.equal(mergedText(parts, ['mine']), '# Courses\n\n- Pain complet');
});
