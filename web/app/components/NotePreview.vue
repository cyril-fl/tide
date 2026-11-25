<script setup lang="ts">
import type { ItemDetail } from '~/types/items';

// A note read only, beside a list (#256): no editing, no tabs, "Open" leads to the editor.
const props = defineProps<{ noteId: string }>();
const { icons } = useIcons();
const api = useApi();
const { starterKit, extensions } = useNoteReader();

const note = ref<ItemDetail>();
const missing = ref(false);
watch(
  () => props.noteId,
  async (id) => {
    note.value = undefined;
    missing.value = false;
    const read = await api<ItemDetail>(`/api/notes/${id}`, { quiet: true }).catch(() => undefined);
    if (props.noteId !== id) return;
    note.value = read;
    missing.value = !read;
  },
  { immediate: true },
);
const content = computed(() => note.value && withoutTitle(note.value.content ?? ''));
</script>

<template>
  <UEmpty
    v-if="missing"
    :icon="icons.noteNotFound"
    :title="$t('note.notFound')"
    :description="$t('note.notFoundDescription')"
    class="m-auto"
  />
  <article v-else-if="note" class="flex min-h-0 flex-1 flex-col">
    <header class="flex items-center gap-2 border-b border-default px-8 py-3">
      <h2 class="truncate text-lg font-semibold">{{ displayTitle(note.item.title, $t('untitled')) }}</h2>
      <UIcon
        v-if="note.readonly"
        :name="icons.lock"
        :aria-label="$t('note.readonly')"
        class="size-4 shrink-0 text-muted"
      />
      <UButton
        :to="`/notes/${noteId}`"
        :label="$t('folder.open')"
        color="neutral"
        variant="subtle"
        size="sm"
        class="ml-auto shrink-0"
      />
    </header>
    <div class="min-h-0 flex-1 overflow-y-auto px-8 py-4">
      <UEditor
        :key="noteId"
        :model-value="content"
        content-type="markdown"
        :editable="false"
        :image="false"
        :mention="false"
        :starter-kit="starterKit"
        :extensions="extensions"
        :ui="{ base: 'px-0 sm:px-0' }"
      />
    </div>
  </article>
</template>
