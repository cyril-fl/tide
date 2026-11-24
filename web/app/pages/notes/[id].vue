<script setup lang="ts">
// Same page for every note: the panels keep their editors from one note to the next.
definePageMeta({ key: 'note' });

const route = useRoute();
const { t } = useI18n();
const tabs = useTabs();
const split = useSplit();
// Below lg the panels stack.
const wide = useMediaQuery('(min-width: 1024px)');

// The address opens its note in the panel showing it, else in the current one; a note not saved refuses (useTabs).
tabs.route(route.params.id as string);
onBeforeRouteUpdate((to) => tabs.route(to.params.id as string));
onUnmounted(useRouter().afterEach((to, _from, failure) => tabs.settle(to.path, !!failure)));
// A narrower screen folds the panels that no longer fit into the last one kept.
watch(split.fits, (max) => tabs.fit(max), { immediate: true });

useHead({
  title: () =>
    displayTitle(tabs.current.value.tabs.find((tab) => tab.id === tabs.current.value.active)?.title, t('untitled')),
});

// The panels' sizes, kept for each number of panels: a new panel takes an even share.
const sizes = useLocalStorage<Record<number, number[]>>('note-panel-sizes', {});
const items = computed(() =>
  split.panels.value.map((panel, index, all) => ({
    id: panel.id,
    slot: 'panel' as const,
    defaultSize: sizes.value[all.length]?.[index] ?? 100 / all.length,
    minSize: 15,
  })),
);
const keep = (layout: number[]) => layout.length > 1 && (sizes.value = { ...sizes.value, [layout.length]: layout });

// The panel keeps the related notes and the graph from one note to the next, not for a note opened long after.
onUnmounted(() => clearNuxtState(['related-shown', 'graph-shown']));
</script>

<template>
  <USplitter
    :items="items"
    :orientation="wide ? 'horizontal' : 'vertical'"
    class="min-h-0 flex-1"
    :ui="{ handle: wide ? 'border-x border-default' : 'border-y border-default' }"
    @layout="keep"
  >
    <template #panel="{ item, index }">
      <NotePanel :panel="item.id" :index="index" :stacked="!wide" />
    </template>
  </USplitter>
</template>
