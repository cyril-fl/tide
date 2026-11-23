<script setup lang="ts">
import type { SavedSearch } from '~/types/items';

const { icons } = useIcons();
const props = defineProps<{ search: SavedSearch; all: SavedSearch[] }>();
const route = useRoute();

const { savedSearchMenu, reorderSavedSearch } = useSavedSearches();

const link = useTemplateRef('link');
useReorderItem(link, () => props.search.id);
const isOver = useDropReorder(link, () => props.search.id, reorderSavedSearch);

const open = ref(true);
const children = computed(() => props.all.filter((s) => s.parent_id === props.search.id));
</script>

<template>
  <UContextMenu :items="savedSearchMenu(search)">
    <!-- A group opens and closes; a smart folder is a link. -->
    <button
      v-if="search.group"
      ref="link"
      type="button"
      class="row w-full"
      :class="{ 'row-over': isOver }"
      :aria-expanded="open"
      @click="open = !open"
    >
      <UIcon :name="open ? icons.chevronDown : icons.chevronRight" class="size-4 shrink-0" />
      <UIcon :name="icons.folder" class="size-4 shrink-0" :class="isOver ? 'text-inverted' : 'text-primary'" />
      <span class="truncate">{{ search.name }}</span>
    </button>
    <NuxtLink
      v-else
      ref="link"
      :to="`/searches/${search.id}`"
      class="row"
      :class="{ 'row-active': route.params.id === search.id, 'row-over': isOver }"
    >
      <UIcon :name="icons.savedSearch" class="size-4 shrink-0" :class="isOver ? 'text-inverted' : 'text-primary'" />
      <span class="truncate">{{ search.name }}</span>
    </NuxtLink>
  </UContextMenu>
  <div v-if="search.group && open" class="ms-3 border-s border-muted ps-1">
    <SavedSearchRow v-for="child in children" :key="child.id" :search="child" :all="all" />
  </div>
</template>
