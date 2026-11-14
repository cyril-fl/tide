<script setup lang="ts">
import type { Editor } from '@tiptap/vue-3';
import type { Command } from '@tiptap/pm/state';
import {
  SearchQuery,
  findNext,
  findPrev,
  getMatchHighlights,
  replaceAll,
  replaceNext,
  setSearchState,
} from 'prosemirror-search';

// The bar of useNoteSearch over the text: every match highlighted, "3/12" for the one selected.
const props = defineProps<{ editor: Editor; readonly?: boolean }>();
const open = defineModel<boolean>('open', { required: true });

const { icons } = useIcons();
const find = ref('');
const replace = ref('');
const counter = ref({ current: 0, total: 0 });

function count() {
  const { state } = props.editor;
  const matches = getMatchHighlights(state).find();
  const { from, to } = state.selection;
  const current = matches.findIndex((match) => match.from === from && match.to === to) + 1;
  counter.value = { current, total: matches.length };
}

function setQuery() {
  const { view } = props.editor;
  view.dispatch(setSearchState(view.state.tr, new SearchQuery({ search: find.value, replace: replace.value })));
}

const run = (command: Command) => command(props.editor.state, props.editor.view.dispatch);

function close() {
  find.value = '';
  setQuery();
  open.value = false;
  props.editor.commands.focus();
}

watch([find, replace], setQuery);
onMounted(() => props.editor.on('transaction', count));
onBeforeUnmount(() => props.editor.off('transaction', count));
</script>

<template>
  <div v-if="open" class="flex shrink-0 flex-wrap items-center gap-1 border-b border-muted px-7 py-1">
    <UInput
      v-model="find"
      autofocus
      size="sm"
      :placeholder="$t('note.search.find')"
      :aria-label="$t('note.search.find')"
      @keydown.enter.exact.prevent="run(findNext)"
      @keydown.enter.shift.prevent="run(findPrev)"
      @keydown.esc.prevent="close"
    >
      <template #trailing>
        <span v-if="find" class="text-xs text-muted tabular-nums">{{ counter.current }}/{{ counter.total }}</span>
      </template>
    </UInput>
    <UButton
      :icon="icons.up"
      size="sm"
      color="neutral"
      variant="ghost"
      :aria-label="$t('note.search.previous')"
      @click="run(findPrev)"
    />
    <UButton
      :icon="icons.down"
      size="sm"
      color="neutral"
      variant="ghost"
      :aria-label="$t('note.search.next')"
      @click="run(findNext)"
    />
    <template v-if="!readonly">
      <UInput
        v-model="replace"
        size="sm"
        :placeholder="$t('note.search.replace')"
        :aria-label="$t('note.search.replace')"
        @keydown.enter.prevent="run(replaceNext)"
        @keydown.esc.prevent="close"
      />
      <UButton
        :label="$t('note.search.replaceOne')"
        size="sm"
        color="neutral"
        variant="ghost"
        @click="run(replaceNext)"
      />
      <UButton
        :label="$t('note.search.replaceAll')"
        size="sm"
        color="neutral"
        variant="ghost"
        @click="run(replaceAll)"
      />
    </template>
    <UButton
      :icon="icons.close"
      size="sm"
      color="neutral"
      variant="ghost"
      class="ml-auto"
      :aria-label="$t('note.search.close')"
      @click="close"
    />
  </div>
</template>
