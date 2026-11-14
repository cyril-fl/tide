<script setup lang="ts">
import type { CommandPaletteGroup, CommandPaletteItem } from '@nuxt/ui';
import type { ItemDetail, Match, Recent, ReplaceResult, User } from '~/types/items';

const { icons } = useIcons();
const { t } = useI18n();

const api = useApi();
const { has } = usePro();
const route = useRoute();
const { createFolder, moveItem, renameFolder, renameTag } = useItems();
const { importNotes, prompt, confirm } = useDialogs();
const toast = useToast();

const open = useState('search-open', () => false);
// The T shortcut opens the palette on the snippets alone.
const snippetsOnly = useState('search-snippets', () => false);
watch(open, (isOpen) => isOpen || (snippetsOnly.value = false));
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

/** Counts first, then replaces once the exact numbers are confirmed: the API changes every note at once. */
async function replaceEverywhere() {
  const find = await prompt({ title: t('search.replace.find'), value: term.value.trim() || undefined });
  if (!find) return;
  const replace = await prompt({ title: t('search.replace.with', { find }), action: t('search.replace.submit') });
  if (!replace) return;
  const replaceNotes = (dryRun: boolean) =>
    api<ReplaceResult>('/api/replace', { method: 'POST', body: { find, replace, dry_run: dryRun } });
  const count = await replaceNotes(true);
  const locked = count.locked ? t('search.replace.locked', { n: count.locked }, count.locked) : '';
  if (!count.notes) {
    toast.add({ title: t('search.replace.none', { find }), description: locked || undefined });
    return;
  }
  const confirmed = await confirm({
    title: t('search.replace.confirm', { find, replace }),
    description: [t('search.replace.notes', { n: count.notes, occurrences: count.occurrences }, count.notes), locked]
      .filter(Boolean)
      .join(' '),
    action: t('search.replace.submit'),
  });
  if (!confirmed) return;
  const done = await replaceNotes(false);
  toast.add({ title: t('search.replace.done', { n: done.notes }, done.notes), color: 'success' });
  await refreshNuxtData();
}

const proActions = computed<CommandPaletteItem[]>(() => {
  if (!has('paletteActions')) return [];
  const target = here.value;
  const actions: CommandPaletteItem[] = [];
  if (target && target.kind !== 'tag') {
    const children = destinations.value.map((folder) => ({
      label: folder.label,
      icon: icons.folder,
      onSelect: () => moveItem(target.id, folder.value === ROOT ? null : folder.value),
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
      ? [{ label: t('search.replace.action'), icon: icons.replace, onSelect: replaceEverywhere }]
      : []),
    { label: t('search.theme'), icon: icons.appearance, children },
  ];
});

const toNote = (note: { id: string; title: string; path: string; readonly?: boolean }): CommandPaletteItem => ({
  label: displayTitle(note.title, t('untitled')),
  suffix: note.path,
  icon: icons.note,
  to: `/notes/${note.id}`,
  readonly: note.readonly,
});

// The API searches the text: the palette shows what it found as is, not filtered again on titles.
const groups = computed<CommandPaletteGroup[]>(() => {
  const searching = term.value.trim() !== '';
  if (snippetsOnly.value) return [{ id: 'snippets', label: t('search.fromSnippet'), items: snippets.value }];
  return [
    searching
      ? { id: 'notes', label: t('common.notes'), items: matches.value.map(toNote), ignoreFilter: true, slot: 'note' }
      : { id: 'recent', label: t('sidebar.recent'), items: recent.value.slice(0, 8).map(toNote), slot: 'note' },
    {
      id: 'actions',
      label: t('search.actions'),
      items: [
        { label: t('common.newNote'), icon: icons.noteAdd, kbds: ['N'], to: newNotePath(currentFolder(route)) },
        ...(snippets.value.length
          ? [{ label: t('search.fromSnippet'), icon: icons.snippet, kbds: ['T'], children: snippets.value }]
          : []),
        {
          label: t('common.newFolder'),
          icon: icons.folderAdd,
          kbds: ['shift', 'N'],
          onSelect: () => createFolder(currentFolder(route)),
        },
        { label: t('settings.import'), icon: icons.import, onSelect: () => importNotes() },
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
  >
    <!-- The padlock of a locked note at the end of its row, where folders and actions show their chevron. -->
    <template #note-trailing="{ item }">
      <UIcon
        v-if="item.readonly"
        :name="icons.lock"
        :aria-label="$t('note.readonly')"
        class="size-3.5 shrink-0 text-muted"
      />
    </template>
  </UDashboardSearch>
</template>
