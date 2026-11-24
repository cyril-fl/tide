import assert from 'node:assert/strict';
import { test } from 'node:test';
import { getSchema } from '@tiptap/core';
import { MarkdownManager } from '@tiptap/markdown';
import { Node } from '@tiptap/pm/model';
import StarterKit from '@tiptap/starter-kit';
import { hashtagRanges } from '../app/utils/hashtag.ts';

const markdown = new MarkdownManager({ extensions: [StarterKit] });
const schema = getSchema([StarterKit]);
const tags = (source: string) => {
  const doc = Node.fromJSON(schema, markdown.parse(source));
  return hashtagRanges(doc).map(([from, to]) => doc.textBetween(from, to));
};

// The editor shows as a tag what the server reads as one (core/services/hashtag).
test('finds the #tags the server reads', () => {
  assert.deepEqual(tags('#idée et #a-b_2, **#gras**'), ['#idée', '#a-b_2', '#gras']);
  assert.deepEqual(tags('# Titre\n\n#123 a#b x/#c &#38; #d#e'), ['#d']);
  assert.deepEqual(tags('`#code` [#lien](https://x.fr) https://x.fr/#ancre\n\n```\n#bloc\n```'), []);
});
