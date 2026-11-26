<script setup lang="ts">
import type { ItemDetail } from '~/types/items';

// A note read only, beside a list (#256), a double click on the list opens it; unlocked, the editor itself takes its place, locked again at the next note.
const props = defineProps<{ noteId: string }>();
const { icons } = useIcons();
const api = useApi();
const { starterKit, extensions } = useNoteReader();

const note = ref<ItemDetail>();
const missing = ref(false);
const editing = ref(false);
async function load(id: string) {
  const read = await api<ItemDetail>(`/api/notes/${id}`, { quiet: true }).catch(() => undefined);
  if (props.noteId !== id) return;
  note.value = read;
  missing.value = !read;
}
watch(
  () => props.noteId,
  (id) => {
    editing.value = false;
    note.value = undefined;
    missing.value = false;
    load(id);
  },
  { immediate: true },
);
// Locked again, the text read shows what was just written.
watch(editing, (on) => !on && load(props.noteId));
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
      <h2 v-if="!editing" class="truncate text-lg font-semibold">
        {{ displayTitle(note.item.title, $t('untitled')) }}
      </h2>
      <UIcon
        v-if="note.readonly"
        :name="icons.lock"
        :aria-label="$t('note.readonly')"
        class="ml-auto size-4 shrink-0 text-muted"
      />
      <UTooltip v-else :text="editing ? $t('folder.previewRead') : $t('folder.previewEdit')">
        <UButton
          :icon="editing ? icons.unlock : icons.lock"
          color="neutral"
          variant="ghost"
          size="sm"
          class="ml-auto shrink-0"
          :aria-label="editing ? $t('folder.previewRead') : $t('folder.previewEdit')"
          :aria-pressed="!editing"
          @click="editing = !editing"
        />
      </UTooltip>
    </header>
    <NoteEditor v-if="editing" :key="noteId" :note-id="noteId" />
    <div v-else class="min-h-0 flex-1 overflow-y-auto px-8 py-4">
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
