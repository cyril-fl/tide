import { Extension, type Range } from '@tiptap/core';
import type { Node as ProseMirrorNode } from '@tiptap/pm/model';
import { Plugin, PluginKey, type EditorState } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import type { Editor } from '@tiptap/vue-3';
import type { ItemDetail, NoteRef } from '~/types/items';
import type { LinkSize } from '~/utils/linkCard';
import LinkCard from '~/components/LinkCard.vue';
import NoteEmbed from '~/components/NoteEmbed.vue';

const ALLOWED_LINKS = ['http', 'https', 'mailto'];

/** The links of the note editor: web links and their cards, [[links]] to the other notes and [[#links]] to its own headings, ![[embeds]] of the other notes. id: the note, left out of the [[ menu. */
export function useNoteLinks(id: Ref<string | undefined>): EditorModule {
  const { icons } = useIcons();
  const { t } = useI18n();
  const api = useApi();
  const { prompt } = useDialogs();
  const toast = useToast();

  async function askLink(editor: Editor) {
    const current = editor.getAttributes('link').href as string | undefined;
    if (current) return editor.chain().focus().unsetLink().run();

    const href = await prompt({
      title: t('note.addLink'),
      placeholder: t('note.linkPlaceholder'),
      action: t('common.add'),
    });
    if (!href) return;
    const { from, to } = editor.state.selection;
    editor
      .chain()
      .focus()
      .insertContent(linkContent(toHref(href), editor.state.doc.textBetween(from, to)))
      .run();
  }

  // A link swapped in place to another size, the same href and text.
  function setLinkSize(editor: Editor, size: LinkSize) {
    const card = editor.isActive('linkCard') && editor.getAttributes('linkCard');
    if (card)
      return editor
        .chain()
        .focus()
        .insertContent(linkContent(card.href, card.text, size));
    if (size === 'hyperlink') return editor.chain();
    const href = editor.getAttributes('link').href as string;
    editor.chain().focus().extendMarkRange('link').run();
    const { from, to } = editor.state.selection;
    return editor.chain().insertContent(linkContent(href, editor.state.doc.textBetween(from, to), size));
  }

  function linkSizeHandler(size: LinkSize) {
    return command(
      (editor) => setLinkSize(editor, size),
      (editor) => (size === 'hyperlink' ? editor.isActive('link') : editor.isActive('linkCard', { size })),
    );
  }

  function linkHref(editor: Editor) {
    return (editor.getAttributes('linkCard').href ?? editor.getAttributes('link').href) as string | undefined;
  }

  // The notes offered after [[, found by the API on the text before #: the whole list weighs 7 MB at 50,000 notes (#150).
  // Picked, the link points to the note's id right away.
  const query = ref('');
  const search = refDebounced(
    computed(() => query.value.split('#')[0]!.trim()),
    150,
  );
  const { data: notes } = useLazyAsyncData(
    'link-notes',
    () => api<NoteRef[]>('/api/notes', { query: { q: search.value }, quiet: true }),
    { watch: [search] },
  );
  const { data: neighbors } = useNeighbors(id);
  const { has } = usePro();
  const noteItems = computed(() =>
    linkTargets(
      notes.value,
      id.value,
      has('linkSuggestions') ? neighbors.value?.neighbors : undefined,
      search.value,
    ).map((note) => ({
      kind: 'wikiLink',
      label: note.title,
      description: note.path,
      icon: icons.note,
      id: note.id,
    })),
  );

  // [[Note# offers the headings of that note, read as soon as the text names it.
  const headingNote = computed(() => {
    const name = query.value.split('#')[0]!.trim().toLowerCase();
    return noteItems.value.find((note) => note.label.toLowerCase() === name);
  });
  const headings = ref<string[]>([]);
  // By id: a save refreshes the notes, the same note comes back as a new item.
  watch(
    () => headingNote.value?.id,
    async (noteId) => {
      headings.value = [];
      if (!noteId) return;
      const detail = await api<ItemDetail>(`/api/notes/${noteId}`, { quiet: true }).catch(() => undefined);
      if (headingNote.value?.id === noteId) headings.value = headingsOf(detail?.content ?? '');
    },
  );
  // [[# offers the headings of the note itself, read from the text being written.
  const ownHeadings = ref<string[]>([]);
  // The menu does not filter its items: it reads them on a key only, before the API answers. The headings are filtered here.
  const linkItems = computed(() => {
    const hash = query.value.indexOf('#');
    if (hash < 0) return noteItems.value;
    const typed = query.value.slice(hash + 1).toLowerCase();
    const holding = (list: string[]) => list.filter((heading) => heading.toLowerCase().includes(typed));
    if (hash === 0)
      return holding(ownHeadings.value).map((heading) => ({
        kind: 'wikiLink',
        label: `#${heading}`,
        icon: icons.heading1,
        id: null,
      }));
    const note = headingNote.value;
    if (!note) return [];
    return holding(headings.value).map((heading) => ({
      ...note,
      label: `${note.label}#${heading}`,
      icon: icons.heading1,
      heading,
    }));
  });

  // The other notes the API would pick from, by path or by title whatever its case, as linknotes.Resolve.
  function candidates(notes: NoteRef[], target: string) {
    return notes.filter(
      (note) =>
        note.id !== id.value &&
        (target.startsWith('/') ? note.path === target : note.title.toLowerCase() === target.toLowerCase()),
    );
  }

  // The target of a waiting [[link]], its heading left out.
  function waitingTarget(node: ProseMirrorNode) {
    return node.type.name === 'wikiLink' && !node.attrs.id ? (node.attrs.target.split('#')[0].trim() as string) : '';
  }

  // The waiting [[links]] of the text: the notes the API would pick from are read for their targets alone.
  const waiting = ref<string[]>([]);
  const { data: pending } = useLazyAsyncData(
    'link-candidates',
    async () =>
      new Map(
        await Promise.all(
          waiting.value.map(
            async (target) =>
              [
                target,
                candidates(await api<NoteRef[]>('/api/notes', { query: { q: target }, quiet: true }), target),
              ] as const,
          ),
        ),
      ),
    { watch: [waiting] },
  );
  const pendingCandidates = (target: string) => pending.value?.get(target) ?? [];

  // A waiting [[link]] that several notes answer gets wikilink-ambiguous, redrawn when the notes change.
  const ambiguousKey = new PluginKey('ambiguousLinks');
  const AmbiguousLinks = Extension.create({
    name: 'ambiguousLinks',
    addProseMirrorPlugins() {
      return [
        new Plugin({
          key: ambiguousKey,
          view: (view) => {
            const collect = (state: EditorState) => {
              const targets = new Set<string>();
              state.doc.descendants((node) => {
                const target = waitingTarget(node);
                if (target) targets.add(target);
              });
              const sorted = [...targets].sort();
              if (sorted.join('\n') !== waiting.value.join('\n')) waiting.value = sorted;
            };
            collect(view.state);
            return {
              update: (view) => collect(view.state),
              destroy: watch(pending, () => view.dispatch(view.state.tr.setMeta(ambiguousKey, true))),
            };
          },
          props: {
            decorations: (state) => {
              const decorations: Decoration[] = [];
              state.doc.descendants((node, pos) => {
                const target = waitingTarget(node);
                if (target && pendingCandidates(target).length > 1)
                  decorations.push(Decoration.node(pos, pos + node.nodeSize, { class: 'wikilink-ambiguous' }));
              });
              return DecorationSet.create(state.doc, decorations);
            },
          },
        }),
      ];
    },
  });

  function openLink(event: MouseEvent) {
    const link = (event.target as HTMLElement).closest<HTMLElement>('a[data-wikilink]');
    if (!link) return;
    event.preventDefault();
    const own = link.dataset.wikilink!.trim();
    if (own.startsWith('#')) {
      const heading = [...link.closest('.ProseMirror')!.querySelectorAll('h1, h2, h3, h4, h5, h6')].find((el) =>
        sameHeading(el.textContent ?? '', own.slice(1)),
      );
      if (heading) return heading.scrollIntoView({ behavior: 'smooth', block: 'start' });
      return toast.add({
        title: t('note.missingHeading'),
        description: t('note.missingHeadingDescription', { target: own.slice(1) }),
        color: 'warning',
      });
    }
    const heading = link.dataset.heading ? `#${encodeURIComponent(link.dataset.heading)}` : '';
    if (link.dataset.note) return navigateTo(`/notes/${link.dataset.note}${heading}`);
    const target = link.dataset.wikilink!.split('#')[0]!.trim();
    const paths = pendingCandidates(target).map((note) => note.path);
    toast.add(
      paths.length > 1
        ? {
            title: t('note.ambiguousLink'),
            description: t('note.ambiguousLinkDescription', { target, paths: paths.join(', ') }),
            color: 'info',
          }
        : {
            title: t('note.pendingLink'),
            description: t('note.pendingLinkDescription', { target }),
            color: 'warning',
          },
    );
  }

  return {
    extensions: [WikiLink, AmbiguousLinks, noteEmbed(NoteEmbed), linkCard(LinkCard)],
    starterKit: {
      link: {
        openOnClick: false,
        // Autolink asks with the text as typed: an email or a domain has no scheme yet, it gets mailto: or https:.
        isAllowedUri: (url: string, ctx: { defaultValidate: (url: string) => boolean }) => {
          const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(url)?.[1];
          return (!scheme || ALLOWED_LINKS.includes(scheme.toLowerCase())) && ctx.defaultValidate(url);
        },
      },
    },
    handlers: {
      // After ![[ the note is embedded: the ! goes with the typed text.
      wikiLink: command((editor, item: { label: string; id: string | null; heading?: string }) => {
        const { from } = editor.state.selection;
        const embed = !!item.id && editor.state.doc.textBetween(Math.max(from - 1, 0), from) === '!';
        return editor
          .chain()
          .focus()
          .deleteRange(embed ? { from: from - 1, to: from } : { from, to: from })
          .insertContent([
            {
              type: embed ? 'noteEmbed' : 'wikiLink',
              attrs: { target: item.label, id: item.id, heading: item.heading ?? null },
            },
            { type: 'text', text: ' ' },
          ]);
      }),
      link: command(
        (editor) => {
          askLink(editor);
          return editor.chain();
        },
        (editor) => editor.isActive('link'),
      ),
      linkHyperlink: linkSizeHandler('hyperlink'),
      linkMini: linkSizeHandler('mini'),
      linkLarge: linkSizeHandler('large'),
      linkOpen: command((editor) => {
        const href = linkHref(editor);
        // A mail link opens the mail app: a new tab would stay empty.
        if (href && isMailLink(href)) window.location.href = href;
        else if (isCardLink(href)) window.open(href, '_blank', 'noopener');
        return editor.chain();
      }),
    },
    // Shown on a web link or a card, like the menu of Apple Notes.
    bubbles: [
      {
        pluginKey: 'linkMenu',
        items: [
          [
            { kind: 'linkHyperlink', icon: icons.link, tooltip: { text: t('note.linkHyperlink') } },
            { kind: 'linkMini', icon: icons.linkMini, tooltip: { text: t('note.linkMini') } },
            { kind: 'linkLarge', icon: icons.linkLarge, tooltip: { text: t('note.linkLarge') } },
          ],
          [{ kind: 'linkOpen', icon: icons.linkOpen, tooltip: { text: t('note.linkOpen') } }],
        ],
        shouldShow: ({ editor }) =>
          editor.isActive('linkCard') || (editor.isActive('link') && isCardLink(editor.getAttributes('link').href)),
      },
    ],
    menus: [
      {
        char: '[[',
        pluginKey: 'wikiLinkMenu',
        items: linkItems,
        // TODO: patches/ makes @nuxt/ui 4.12 pass ignoreFilter to its menu, drop the patch once Nuxt UI does.
        ignoreFilter: true,
        suggestion: {
          allowSpaces: true,
          // ![[ embeds the note picked.
          allowedPrefixes: [' ', '!'],
          // The typed text is read here, for the search and the headings after #.
          allow: ({ state, range }: { state: EditorState; range: Range }) => {
            query.value = state.doc.textBetween(range.from + 2, range.to);
            if (query.value.startsWith('#')) {
              const headings: string[] = [];
              state.doc.forEach((node) => {
                if (node.type.name === 'heading' && node.textContent) headings.push(node.textContent);
              });
              if (headings.join('\n') !== ownHeadings.value.join('\n')) ownHeadings.value = headings;
            }
            // A ] closes the link typed by hand: the menu closes, or it would leave brackets behind ![[Note]].
            return !query.value.includes(']');
          },
        },
      },
    ],
    onClick: openLink,
  };
}
