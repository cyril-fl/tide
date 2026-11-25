<script setup lang="ts">
import type { Match } from '~/types/items';

const { icons } = useIcons();

const route = useRoute();
const api = useApi();
const view = useView();
const { active: previewing } = usePreview();

const q = computed(() => (typeof route.query.q === 'string' ? route.query.q : ''));

const {
  items: matches,
  status,
  load,
} = usePages(
  () => `search:q:${q.value}`,
  (query, onResponse) =>
    q.value.trim() ? api<Match[]>('/api/search', { query: { q: q.value, ...query }, onResponse }) : Promise.resolve([]),
);
</script>

<template>
  <ListPreview>
    <div class="flex flex-1 flex-col p-8">
      <UPageHeader :title="$t('search.title')" :headline="q">
        <template #links>
          <ViewToggle />
        </template>
      </UPageHeader>
      <NoteList v-if="matches.length" :notes="matches" :grid="view === 'grid' && !previewing" dated :load="load" />
      <UEmpty
        v-else-if="status !== 'pending'"
        :icon="icons.searchEmpty"
        :title="q.trim() ? $t('search.noResult') : $t('search.empty')"
        :description="q.trim() ? $t('search.noResultDescription', { q }) : $t('search.emptyDescription')"
      />
    </div>
  </ListPreview>
</template>
