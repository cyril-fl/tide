import assert from 'node:assert/strict';
import { test } from 'node:test';
import { MarkdownManager } from '@tiptap/markdown';
import StarterKit from '@tiptap/starter-kit';
import { BlockComment, InlineComment, readBlockComment, readInlineComment } from '../app/utils/comment.ts';

const markdown = new MarkdownManager({ extensions: [StarterKit, InlineComment, BlockComment] });
const roundTrip = (source: string) => markdown.serialize(markdown.parse(source)).trim();

test('reads a comment of either syntax at the start only', () => {
  assert.equal(readInlineComment('%% note %% suite')?.raw, '%% note %%');
  assert.equal(readInlineComment('<!-- a --> b')?.raw, '<!-- a -->');
  assert.equal(readInlineComment('voir %% x %%'), undefined);
  assert.equal(readInlineComment('%%%%'), undefined);
});

// One comment never swallows the text up to the next one.
test('reads a block comment only when alone on its lines', () => {
  assert.equal(readBlockComment('%%\nune\n\ndeux\n%%\n\nsuite')?.text, '%%\nune\n\ndeux\n%%');
  assert.equal(readBlockComment('%% a %% b\n%% c %%'), undefined);
  assert.equal(readBlockComment('<!-- a -->\ntexte')?.text, '<!-- a -->');
});

// Saved then opened again, a note keeps its comments as written, hidden in the editor but not lost.
test('keeps the comments of a note', () => {
  for (const source of [
    'a %% caché %% b',
    'a <!-- caché --> b',
    '# Titre\n\n%%\nplusieurs\n\nlignes\n%%\n\nsuite',
    '<!-- bloc -->\n\nsuite',
    '`%% code %%`',
  ])
    assert.equal(roundTrip(source), source);
});

test('shows nothing of a comment', () => {
  const doc = markdown.parse('a %% caché %% b\n\n%% bloc %%');
  assert.deepEqual(
    doc.content?.map((node) => node.type),
    ['paragraph', 'blockComment'],
  );
});
