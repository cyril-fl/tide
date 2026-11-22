<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui';
import type { Match, SavedSearch, Tag, TagDetail } from '~/types/items';

const { icons } = useIcons();

const route = useRoute();
const api = useApi();
const { t } = useI18n();
const { renameTag, removeTag, exportItems } = useItems();
const { addSavedSearch } = useSavedSearches();
const { has } = usePro();
const view = useView();
const { data: tags } = useTags();

// The sidebar's tags build the filter; one tag alone keeps the tag's own page, with rename and delete.
const filter = computed(() => readTagFilter(route));
const single = computed(() => (filter.value.and.length === 1 && !filter.value.not.length ? filter.value.and[0]! : ''));
const title = computed(() =>
  [...filter.value.and.map((tag) => `#${tag}`), ...filter.value.not.map((tag) => `−#${tag}`)].join(' '),
);

// The tags of the account by value: the search and a smart folder name them by id.
async function tagIds(values: string[]) {
  const all = tags.value.length ? tags.value : await api<Tag[]>('/api/tags');
  return values.flatMap((value) => all.find((tag) => tag.value === value)?.id ?? []);
}

async function read(query: { limit: number; offset: number }, onResponse?: (context: { response: Response }) => void) {
  const { and, not } = filter.value;
  if (single.value)
    return (await api<TagDetail>(`/api${tagPath(single.value)}`, { query, onResponse, quiet: true })).notes;
  const included = await tagIds(and);
  // A tag included that no note carries leaves no note, where its missing id would leave the filter out.
  if (included.length < and.length) return [];
  return api<Match[]>('/api/search', {
    query: { tags_all: included, tags_none: await tagIds(not), ...query },
    onResponse,
  });
}

const {
  items: notes,
  status,
  error,
  load,
} = usePages<Match | TagDetail['notes'][number]>(
  () => `tags:${filter.value.and.join(',')}:${filter.value.not.join(',')}`,
  read,
);

const menu = computed<DropdownMenuItem[][]>(() => [
  [
    {
      label: t('tag.export'),
      icon: icons.export,
      // Every note of the filter, not only the pages read: a limit of 0 reads them all.
      onSelect: async () => exportItems((await read({ limit: 0, offset: 0 })).map((note) => note.id)),
    },
    ...(has('savedSearches')
      ? [
          {
            label: t('tag.saveSearch'),
            icon: icons.savedSearch,
            onSelect: async () =>
              addSavedSearch({
                name: title.value,
                tag_ids: await tagIds(filter.value.and),
                tag_mode: 'all',
                no_tag_ids: await tagIds(filter.value.not),
              } as SavedSearch),
          },
        ]
      : []),
  ],
  ...(single.value
    ? [
        [
          { label: t('common.rename'), icon: icons.rename, onSelect: () => renameTag(single.value) },
          {
            label: t('common.delete'),
            icon: icons.delete,
            color: 'error' as const,
            onSelect: () => removeTag(single.value),
          },
        ],
      ]
    : []),
]);
useHead({ title });
</script>

<template>
  <div class="p-8">
    <PageHeader
      :title="title"
      :subtitle="notes.length || status === 'success' ? $t('tag.count', notes.length) : undefined"
    >
      <ViewToggle />
      <UDropdownMenu :items="menu" :content="{ align: 'end' }">
        <UTooltip :text="$t('tag.actions')">
          <UButton :icon="icons.more" color="neutral" variant="ghost" :aria-label="$t('tag.actions')" />
        </UTooltip>
      </UDropdownMenu>
      <UTooltip :text="$t('tag.newNote')">
        <UButton
          :icon="icons.noteAdd"
          color="neutral"
          variant="ghost"
          :aria-label="$t('tag.newNote')"
          :to="newNotePath(undefined, undefined, filter.and)"
        />
      </UTooltip>
    </PageHeader>
    <NoteList v-if="notes.length" :notes="notes" :grid="view === 'grid'" dated :load="load" />
    <UEmpty
      v-else-if="error"
      :icon="icons.tag"
      :title="$t('tag.notFound')"
      :description="$t('tag.notFoundDescription')"
      :actions="[{ label: $t('common.backToNotes'), icon: icons.home, color: 'neutral', variant: 'subtle', to: '/' }]"
    />
    <UEmpty
      v-else-if="status === 'success' && single"
      :icon="icons.tag"
      :title="$t('tag.empty')"
      :description="$t('tag.emptyDescription', { value: single })"
    />
    <UEmpty
      v-else-if="status === 'success'"
      :icon="icons.tag"
      :title="$t('tag.filterEmpty')"
      :description="$t('tag.filterEmptyDescription')"
    />
  </div>
</template>
