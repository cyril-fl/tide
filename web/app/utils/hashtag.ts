import { Extension } from '@tiptap/core';
import type { Node } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';

// The rule of core/services/hashtag: # then letters, digits, _ or - with a letter, not glued to one of them, #, \, &, /.
const word = /(?<![\p{L}\p{N}_#\\&/-])#[\p{L}\p{N}_-]*\p{L}[\p{L}\p{N}_-]*/gu;

/** The [from, to[ of each #tag of the doc read as a tag: not in code nor in a link (wiki links are nodes of their own). */
export function hashtagRanges(doc: Node): [number, number][] {
  const ranges: [number, number][] = [];
  doc.descendants((node, pos) => {
    if (node.type.spec.code) return false;
    if (!node.isText || node.marks.some((mark) => mark.type.spec.code || mark.type.name === 'link')) return;
    for (const match of node.text!.matchAll(word))
      ranges.push([pos + match.index, pos + match.index + match[0].length]);
  });
  return ranges;
}

function decorate(doc: Node) {
  return DecorationSet.create(
    doc,
    hashtagRanges(doc).map(([from, to]) => Decoration.inline(from, to, { class: 'hashtag' })),
  );
}

const key = new PluginKey<DecorationSet>('hashtag');

/** Shows the #tags of the text as the tags of the sidebar; they stay plain text, edited like any word. */
export const Hashtag = Extension.create({
  name: 'hashtag',
  addProseMirrorPlugins() {
    return [
      new Plugin({
        key,
        state: {
          init: (_, state) => decorate(state.doc),
          apply: (tr, old) => (tr.docChanged ? decorate(tr.doc) : old),
        },
        props: { decorations: (state) => key.getState(state) },
      }),
    ];
  },
});
