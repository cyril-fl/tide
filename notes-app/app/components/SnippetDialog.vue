<script setup lang="ts">
import type { ItemDetail, NoteRef, Snippet } from '~/types/items';

const props = defineProps<{ title: string; value?: Snippet }>();
const emit = defineEmits<{ close: [value?: Snippet] }>();

const { icons } = useIcons();
const { has } = usePro();
const api = useApi();

const name = ref(props.value?.name ?? '');
const content = ref(props.value?.content ?? '');
// The editor keeps its own text: handing back what it writes would start it again on every key.
// It only reads start, which the source mode, written as plain text, sets back on its way out.
const source = ref(false);
const start = ref(content.value);
function toggleSource() {
  if (source.value) start.value = content.value;
  source.value = !source.value;
}

// The notes offered to start from, found by the API like the [[links]]: the whole list is too heavy.
const query = ref('');
const search = refDebounced(query, 150);
const { data: notes } = useLazyAsyncData(
  'snippet-notes',
  () => api<NoteRef[]>('/api/notes', { query: { q: search.value }, quiet: true }),
  { watch: [search], default: () => [] as NoteRef[] },
);

// Picked, the note's text replaces the content, without its title: the name holds it when still empty.
async function fromNote(note?: NoteRef) {
  if (!note) return;
  const detail = await api<ItemDetail>(`/api/notes/${note.id}`);
  content.value = start.value = withoutTitle(detail.content ?? '');
  if (!name.value.trim()) name.value = note.title;
}

function submit() {
  if (name.value.trim()) emit('close', { name: name.value.trim(), content: content.value });
}
</script>

<template>
  <UModal :title="title" :close="false" :ui="{ content: 'max-w-2xl' }" @update:open="emit('close')">
    <template #body>
      <form id="snippet" class="flex flex-col gap-4" @submit.prevent="submit">
        <UInput
          v-model="name"
          autofocus
          class="w-full"
          :placeholder="$t('settings.snippetName')"
          :aria-label="$t('settings.snippetName')"
        />
        <div v-if="source" class="flex h-[32rem] max-h-[60vh] flex-col rounded-md ring ring-accented">
          <LazyNoteSource v-model="content" compact :label="$t('note.source')" />
        </div>
        <!-- The box holds the height: the editor fills it, flex-1 would ignore a height of its own. -->
        <div v-else class="flex h-[32rem] max-h-[60vh] flex-col rounded-md ring ring-accented">
          <NoteWriter
            :model-value="start"
            compact
            :label="$t('settings.snippetContent')"
            @update:model-value="content = $event"
          />
        </div>
      </form>
    </template>
    <template #footer>
      <div class="flex w-full items-center gap-2">
        <UTooltip v-if="has('sourceMode')" :text="source ? $t('note.editor') : $t('note.source')">
          <UButton
            :icon="icons.source"
            color="neutral"
            variant="ghost"
            size="xs"
            :aria-label="$t('note.source')"
            :aria-pressed="source"
            @click="toggleSource"
          />
        </UTooltip>
        <UInputMenu
          v-model:search-term="query"
          :items="notes"
          label-key="title"
          ignore-filter
          :icon="icons.note"
          size="xs"
          class="w-56"
          :placeholder="$t('settings.snippetFromNote')"
          :aria-label="$t('settings.snippetFromNote')"
          @update:model-value="fromNote"
        />
        <UButton class="ms-auto" :label="$t('common.cancel')" color="neutral" variant="ghost" @click="emit('close')" />
        <UButton type="submit" form="snippet" :label="$t('settings.save')" :disabled="!name.trim()" />
      </div>
    </template>
  </UModal>
</template>
