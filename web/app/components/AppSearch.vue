<script setup lang="ts">
import type { CommandPaletteGroup, CommandPaletteItem, DropdownMenuItem } from '@nuxt/ui';
import type { Item, ItemDetail, Match, Recent, User } from '~/types/items';

const { icons } = useIcons();
const { shortcuts, key } = useShortcuts();
const { t } = useI18n();

const api = useApi();
const { has } = usePro();
const route = useRoute();
const { createFolder, itemMenu, moveItem, renameFolder, renameTag } = useItems();
const { importNotes, replaceNotes } = useDialogs();

const open = useState('search-open', () => false);
// The Option+T shortcut opens the palette on the snippets alone.
const snippetsOnly = useState('search-snippets', () => false);
// "Open beside" turns the palette into a pick of the note opened beside, by the same search (#128).
const besideOnly = ref(false);
const split = useSplit();
watch(open, (isOpen) => isOpen || (snippetsOnly.value = besideOnly.value = false));
const term = ref('');
const query = refDebounced(term, 200);

const { data: matches, status } = useLazyAsyncData(
  'palette-search',
  () =>
    // The first matches only: "all results" opens /search for the rest.
    query.value.trim()
      ? api<Match[]>('/api/search', { query: { q: query.value, limit: 20 }, quiet: true })
      : Promise.resolve([]),
  { watch: [query], default: () => [] as Match[] },
);
const { data: recent } = useLazyAsyncData('recent', () => api<Recent[]>('/api/recent', { quiet: true }), {
  default: () => [] as Recent[],
  dedupe: 'defer',
});

const { data: me } = useMe();
const snippets = computed<CommandPaletteItem[]>(() =>
  (me.value?.snippets ?? []).map((snippet, index) => ({
    label: snippet.name,
    icon: icons.snippet,
    to: newNotePath(currentFolder(route), index),
  })),
);

// The note, folder or tag the page shows, that the Pro actions apply to.
const here = computed(() => {
  const id = route.params.id as string;
  if (route.path.startsWith('/notes/') && id !== 'new') return { kind: 'note' as const, id };
  if (route.path.startsWith('/folders/')) return { kind: 'folder' as const, id };
  if (route.path.startsWith('/tags/')) return { kind: 'tag' as const, id: route.params.value as string };
  return undefined;
});
// Set by the open note's editor, which renames it.
const renameNote = useState<(() => Promise<void>) | undefined>('rename-note');
const destinations = useDestinations(() => (here.value?.kind === 'folder' ? [here.value.id] : []));

async function changeTheme(theme: string) {
  me.value = await api<User>('/api/me', { method: 'PATCH', body: { theme } });
}

// Where the note or folder shown is, for ⌘Z to put it back: the folder page has read it, a note is read here.
async function parentOf(target: { kind: 'note' | 'folder'; id: string }) {
  const read =
    target.kind === 'folder'
      ? useNuxtData<ItemDetail>(`folder:${target.id}`).data.value
      : await api<ItemDetail>(`/api/notes/${target.id}`);
  return read?.item.parent_id ?? null;
}

const proActions = computed<CommandPaletteItem[]>(() => {
  if (!has('paletteActions')) return [];
  const target = here.value;
  const actions: CommandPaletteItem[] = [];
  if (target && target.kind !== 'tag') {
    const children = destinations.value.map((folder) => ({
      label: folder.label,
      icon: icons.folder,
      onSelect: async () => moveItem(target.id, folder.value === ROOT ? null : folder.value, await parentOf(target)),
    }));
    actions.push({ label: t('search.moveTo'), icon: icons.move, children });
  }
  if (target?.kind === 'note' && renameNote.value)
    actions.push({ label: t('common.rename'), icon: icons.rename, onSelect: renameNote.value });
  const folder = target?.kind === 'folder' && useNuxtData<ItemDetail>(`folder:${target.id}`).data.value?.item;
  if (folder) actions.push({ label: t('common.rename'), icon: icons.rename, onSelect: () => renameFolder(folder) });
  if (target?.kind === 'tag')
    actions.push({ label: t('common.rename'), icon: icons.rename, onSelect: () => renameTag(target.id) });
  const children = themes.map((theme) => ({
    label: t(`settings.themes.${theme.id}`),
    icon: me.value?.theme === theme.id ? icons.current : icons.appearance,
    onSelect: () => changeTheme(theme.id),
  }));
  return [
    ...actions,
    ...(has('replaceEverywhere')
      ? [
          {
            label: t('search.replace.action'),
            icon: icons.replace,
            onSelect: () => replaceNotes(term.value.trim() || undefined),
          },
        ]
      : []),
    { label: t('search.theme'), icon: icons.appearance, children },
  ];
});

const toNote = (note: Match | Recent): CommandPaletteItem => ({
  label: displayTitle(note.title, t('untitled')),
  suffix: note.path,
  icon: icons.note,
  ...(besideOnly.value ? { onSelect: () => split.openOther(note.id) } : { to: `/notes/${note.id}` }),
  readonly: note.readonly,
  note: besideOnly.value ? undefined : note,
});

// A note found has the menu of its right-click in the lists: its … button, a right-click on it, or → on it.
const menuFor = ref<string>();
watch(open, () => (menuFor.value = undefined));
function noteMenu(note: Match | Recent): DropdownMenuItem[][] {
  const item = { ...note, kind: 'note', created_at: '' } as Item;
  return itemMenu(item, () => parentOf({ kind: 'note', id: note.id })).map((group) =>
    group.map((entry) => ({
      ...entry,
      onSelect: (event: Event) => {
        open.value = false;
        entry.onSelect?.(event);
      },
    })),
  );
}
const menuOf = (row: Element | null | undefined) =>
  row?.querySelector<HTMLElement>('[data-note-menu]')?.dataset.noteMenu;
useEventListener(document, 'contextmenu', (event) => {
  const id = open.value && menuOf((event.target as Element).closest('[role="option"]'));
  if (!id) return;
  event.preventDefault();
  menuFor.value = id;
});
onKeyStroke('ArrowRight', (event) => {
  const input = event.target as HTMLInputElement;
  if (!open.value || menuFor.value || input.selectionStart !== input.value?.length) return;
  const id = menuOf(document.querySelector('[role="option"][data-highlighted]'));
  if (!id) return;
  event.preventDefault();
  menuFor.value = id;
});
// Closed without an action, the menu gives the keys back to the search, on its note again: the focus gone into the
// menu, out of the list, took the highlight away, and the list highlights a row on the pointer or the arrows only.
let lastMenu: string | undefined;
watch(menuFor, (id) => id && (lastMenu = id));
function backToSearch(event: Event) {
  event.preventDefault();
  document.querySelector<HTMLInputElement>('[role="dialog"] input')?.focus();
  document
    .querySelector(`[data-note-menu="${lastMenu}"]`)
    ?.closest('[role="option"]')
    ?.dispatchEvent(new PointerEvent('pointermove', { bubbles: true }));
}
// Kept open (preventDefault) on the notes to pick, the search typed so far cleared.
const openBeside = (event: Event) => {
  event.preventDefault();
  besideOnly.value = true;
  term.value = '';
};

// The API searches the text: the palette shows what it found as is, not filtered again on titles.
const groups = computed<CommandPaletteGroup[]>(() => {
  const searching = term.value.trim() !== '';
  if (snippetsOnly.value) return [{ id: 'snippets', label: t('search.fromSnippet'), items: snippets.value }];
  const notes = searching
    ? { id: 'notes', label: t('common.notes'), items: matches.value.map(toNote), ignoreFilter: true, slot: 'note' }
    : { id: 'recent', label: t('sidebar.recent'), items: recent.value.slice(0, 8).map(toNote), slot: 'note' };
  if (besideOnly.value) return [{ ...notes, label: t('note.openBeside') }];
  return [
    notes,
    {
      id: 'actions',
      label: t('search.actions'),
      items: [
        {
          label: t('common.newNote'),
          icon: icons.noteAdd,
          kbds: shortcuts.newNote,
          to: newNotePath(currentFolder(route)),
        },
        ...(snippets.value.length
          ? [
              {
                label: t('search.fromSnippet'),
                icon: icons.snippet,
                kbds: shortcuts.fromSnippet,
                children: snippets.value,
              },
            ]
          : []),
        {
          label: t('common.newFolder'),
          icon: icons.folderAdd,
          kbds: shortcuts.newFolder,
          onSelect: () => createFolder(currentFolder(route)),
        },
        { label: t('settings.import'), icon: icons.import, onSelect: () => importNotes() },
        ...(split.can.value && here.value?.kind === 'note'
          ? [{ label: t('search.openBeside'), icon: icons.beside, onSelect: openBeside }]
          : []),
        ...proActions.value,
        ...(has('graph') ? [{ label: t('common.graph'), icon: icons.graph, to: '/graph' }] : []),
        { label: t('common.trash'), icon: icons.trash, to: '/trash' },
        { label: t('common.settings'), icon: icons.settings, to: '/settings' },
        ...(searching
          ? [
              {
                label: t('search.allResults', { term: term.value }),
                icon: icons.search,
                to: { path: '/search', query: { q: term.value } },
              },
            ]
          : []),
      ],
    },
  ];
});
</script>

<template>
  <UDashboardSearch
    v-model:open="open"
    v-model:search-term="term"
    :groups="groups"
    :loading="status === 'pending'"
    :placeholder="$t('search.placeholder')"
    :shortcut="key('search')"
  >
    <!-- The padlock of a locked note at the end of its row, where folders and actions show their chevron, then its menu. -->
    <template #note-trailing="{ item }">
      <UIcon
        v-if="item.readonly"
        :name="icons.lock"
        :aria-label="$t('note.readonly')"
        class="size-3.5 shrink-0 text-muted"
      />
      <UDropdownMenu
        v-if="item.note"
        :items="noteMenu(item.note)"
        :open="menuFor === item.note.id"
        :content="{ align: 'end', onCloseAutoFocus: backToSearch }"
        @update:open="menuFor = $event ? item.note.id : undefined"
      >
        <UButton
          :icon="icons.more"
          color="neutral"
          variant="ghost"
          size="xs"
          :aria-label="$t('search.actions')"
          :data-note-menu="item.note.id"
          class="-my-1 opacity-0 group-hover:opacity-100 group-data-highlighted:opacity-100 pointer-coarse:opacity-100"
          @click.stop
          @pointerdown.stop
        />
      </UDropdownMenu>
    </template>
  </UDashboardSearch>
</template>
