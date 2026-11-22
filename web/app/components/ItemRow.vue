<script setup lang="ts">
import type { Item } from '~/types/items';

const { icons } = useIcons();
const { locale } = useI18n();

// text: what a tile shows of a note (a search's snippet); the start of the note when none, or only the title line.
const props = defineProps<{ item: Item; grid?: boolean; selected?: boolean; text?: string }>();

const { itemMenu } = useItems();

const link = useTemplateRef('link');
useDragItem(link, () => props.item);

const isFolder = computed(() => props.item.kind === 'folder');
const to = computed(() => `/${isFolder.value ? 'folders' : 'notes'}/${props.item.id}`);
// Only folders take drops: a note target always refuses.
const isOver = useDropFolder(
  computed(() => (isFolder.value ? link.value : null)),
  () => props.item.id,
);
const given = computed(() => withoutTitle(props.text ?? ''));
const preview = useNotePreview(() =>
  props.grid && !isFolder.value && !given.value
    ? { id: props.item.id, version: props.item.updated_at ?? props.item.created_at }
    : undefined,
);
const text = computed(() => given.value || preview.value);
const date = computed(() =>
  new Date(props.item.updated_at ?? props.item.created_at).toLocaleDateString(locale.value, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }),
);
</script>

<template>
  <UContextMenu :items="itemMenu(item)">
    <NuxtLink v-if="grid" ref="link" :to="to" class="group flex min-w-0 flex-col gap-2">
      <div
        class="tile relative"
        :class="{ 'items-start justify-start': text, 'tile-over': isOver, 'tile-selected': selected && !isOver }"
      >
        <p v-if="text" class="line-clamp-6 text-muted" :class="{ 'pr-5': item.readonly }">{{ text }}</p>
        <UIcon v-else :name="isFolder ? icons.folder : icons.note" class="size-10 text-dimmed" />
        <UIcon
          v-if="item.readonly"
          :name="icons.lock"
          :aria-label="$t('note.readonly')"
          class="absolute top-2 right-2 size-4 text-muted"
        />
      </div>
      <div class="min-w-0 text-center text-xs">
        <p class="truncate">{{ displayTitle(item.title, $t('untitled')) }}</p>
        <p v-if="item.created_at" class="text-muted">{{ date }}</p>
      </div>
    </NuxtLink>
    <NuxtLink
      v-else
      ref="link"
      :to="to"
      class="row h-9"
      :class="{ 'row-over': isOver, 'row-selected': selected && !isOver }"
    >
      <UIcon
        :name="isFolder ? icons.folder : icons.note"
        class="size-4 shrink-0"
        :class="isOver ? 'text-inverted' : isFolder ? 'text-primary' : 'text-muted'"
      />
      <span class="truncate">{{ displayTitle(item.title, $t('untitled')) }}</span>
      <UIcon
        v-if="item.readonly"
        :name="icons.lock"
        :aria-label="$t('note.readonly')"
        class="size-3.5 shrink-0"
        :class="isOver ? 'text-inverted' : 'text-muted'"
      />
      <span class="ml-auto shrink-0 pl-4 text-xs" :class="isOver ? 'text-inverted' : 'text-muted'">{{ date }}</span>
    </NuxtLink>
  </UContextMenu>
</template>
