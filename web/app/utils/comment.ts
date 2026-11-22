import { Node, nodeInputRule } from '@tiptap/core';

// %%…%% as in Obsidian, <!--…--> as in HTML; tempered so one comment never runs into the next.
const COMMENT = String.raw`(?:%%(?:(?!%%)[\s\S])+?%%|<!--(?:(?!-->)[\s\S])*?-->)`;
const INLINE = new RegExp(`^${COMMENT}`);
// Alone on its lines: a comment followed by text on its line is an inline one, in the paragraph.
const BLOCK = new RegExp(`^${COMMENT}[ \\t]*(?:\\n+|$)`);

/** The comment at the start of src, inside a line, as a markdown token. */
export function readInlineComment(src: string) {
  const match = src.match(INLINE);
  return match ? { type: 'inlineComment', raw: match[0] } : undefined;
}

/** The comment alone on its lines at the start of src, on one line or several. */
export function readBlockComment(src: string) {
  const match = src.match(BLOCK);
  return match ? { type: 'blockComment', raw: match[0], text: match[0].trimEnd() } : undefined;
}

// Kept as written in the markdown, shown nowhere: an empty element the cursor steps over.
// Empty rather than display:none, which leaves the caret before it after typing one.
const hidden = { 'data-comment': '', contenteditable: 'false' };

/** A comment inside a line; typing %%text%% or <!--text--> makes one, gone from sight at once. */
export const InlineComment = Node.create({
  name: 'inlineComment',
  inline: true,
  group: 'inline',
  atom: true,
  selectable: false,
  addAttributes: () => ({ raw: { default: '', parseHTML: (el) => el.getAttribute('data-raw') } }),
  parseHTML: () => [{ tag: 'span[data-comment]' }],
  renderHTML: ({ node }) => ['span', { ...hidden, 'data-raw': node.attrs.raw }],
  renderText: () => '',

  markdownTokenizer: {
    name: 'inlineComment',
    level: 'inline',
    start: (src) => src.search(/%%|<!--/),
    tokenize: (src) => readInlineComment(src),
  },
  parseMarkdown: (token, helpers) => helpers.createNode('inlineComment', { raw: token.raw }),
  renderMarkdown: (node) => node.attrs?.raw ?? '',

  addInputRules() {
    // No group in find: nodeInputRule would put the node in place of the group alone.
    return [
      nodeInputRule({ find: /%%(?:(?!%%)[^\n])+%%$/, type: this.type, getAttributes: (m) => ({ raw: m[0] }) }),
      nodeInputRule({ find: /<!--(?:(?!-->)[^\n])*-->$/, type: this.type, getAttributes: (m) => ({ raw: m[0] }) }),
    ];
  },
});

/** A comment alone on its lines, several lines long or not. */
export const BlockComment = Node.create({
  name: 'blockComment',
  group: 'block',
  atom: true,
  selectable: false,
  addAttributes: () => ({ raw: { default: '', parseHTML: (el) => el.getAttribute('data-raw') } }),
  parseHTML: () => [{ tag: 'div[data-comment]' }],
  renderHTML: ({ node }) => ['div', { ...hidden, 'data-raw': node.attrs.raw }],
  renderText: () => '',

  markdownTokenizer: {
    name: 'blockComment',
    level: 'block',
    // Only at the start of a line: marked cuts a paragraph where start says.
    start: (src) => src.search(/^(?:%%|<!--)/m),
    tokenize: (src) => readBlockComment(src),
  },
  parseMarkdown: (token, helpers) => helpers.createNode('blockComment', { raw: token.text }),
  renderMarkdown: (node) => node.attrs?.raw ?? '',
});
