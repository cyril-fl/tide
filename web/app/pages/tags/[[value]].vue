<script setup lang="ts">
import type { Match, Tag, TagDetail } from '~/types/items';

const { icons } = useIcons();

const route = useRoute();
const api = useApi();
const { renameTag, removeTag } = useItems();
const view = useView();
const { data: tags } = useTags();

// The sidebar's tags build the filter; one tag alone keeps the tag's own page, its dates, rename and delete.
const filter = computed(() => readTagFilter(route));
const single = computed(() => (filter.value.and.length === 1 && !filter.value.not.length ? filter.value.and[0] : ''));
const title = computed(() =>
  [...filter.value.and.map((tag) => `#${tag}`), ...filter.value.not.map((tag) => `−#${tag}`)].join(' '),
);

const {
  items: notes,
  status,
  error,
  load,
} = usePages<Match | TagDetail['notes'][number]>(
  () => `tags:${filter.value.and.join(',')}:${filter.value.not.join(',')}`,
  async (query, onResponse) => {
    const { and, not } = filter.value;
    if (single.value)
      return (await api<TagDetail>(`/api${tagPath(single.value)}`, { query, onResponse, quiet: true })).notes;
    // The search matches tags by id: the list may not be read yet on a page opened from its address.
    const all = tags.value.length ? tags.value : await api<Tag[]>('/api/tags');
    const ids = (values: string[]) => values.flatMap((value) => all.find((tag) => tag.value === value)?.id ?? []);
    // A tag included that no note carries leaves no note, where its missing id would leave the filter out.
    if (ids(and).length < and.length) return [];
    return api<Match[]>('/api/search', { query: { tags_all: ids(and), tags_none: ids(not), ...query }, onResponse });
  },
);
useHead({ title });
</script>

<template>
  <div class="p-8">
    <PageHeader
      :title="title"
      :subtitle="notes.length || status === 'success' ? $t('tag.count', notes.length) : undefined"
    >
      <ViewToggle />
      <template v-if="single">
        <UTooltip :text="$t('common.rename')">
          <UButton
            :icon="icons.rename"
            color="neutral"
            variant="ghost"
            :aria-label="$t('tag.renameLabel')"
            @click="renameTag(single)"
          />
        </UTooltip>
        <UTooltip :text="$t('tag.deleteTooltip')">
          <UButton
            :icon="icons.delete"
            color="neutral"
            variant="ghost"
            :aria-label="$t('tag.deleteLabel')"
            @click="removeTag(single)"
          />
        </UTooltip>
      </template>
    </PageHeader>
    <NoteList v-if="notes.length" :notes="notes" :grid="view === 'grid'" :dated="!!single" :load="load" />
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
