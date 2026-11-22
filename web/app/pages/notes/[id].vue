<script setup lang="ts">
import type { ItemDetail } from '~/types/items';

// Same page for every note: a new note getting its address must keep its editor, and the cursor in it.
definePageMeta({ key: 'note' });

const route = useRoute();
const api = useApi();
const { data: me } = useMe();

function editorFor() {
  const id = route.params.id as string;
  // TODO: a reload of /notes/new?snippet= before me is loaded starts empty, wait for me if that bites.
  const tags = typeof route.query.tags === 'string' && route.query.tags ? route.query.tags.split(',') : [];
  const start =
    id !== 'new'
      ? undefined
      : (me.value?.snippets[Number(route.query.snippet)]?.content ??
        // An empty first line above the tags takes the cursor and the title.
        (tags.length ? `&nbsp;\n\n${tags.map((tag) => `#${tag}`).join(' ')}` : undefined));
  return { noteId: id === 'new' ? undefined : id, parentId: route.query.parent as string | undefined, start };
}

const editor = ref<ReturnType<typeof editorFor> & { key: number; note?: ItemDetail }>({ key: 0, ...editorFor() });
let createdId: string | undefined;
let opening = 0;

// The next note is read before its editor replaces the one shown, or the page blinks empty while it loads (#154).
// Not read, it loads itself and says why. Only the last note asked for opens.
watch(
  () => route.fullPath,
  async () => {
    const created = route.params.id === createdId;
    createdId = undefined;
    if (created) return;
    const turn = ++opening;
    const next = editorFor();
    const note = next.noteId
      ? await api<ItemDetail>(`/api/notes/${next.noteId}`, { quiet: true }).catch(() => undefined)
      : undefined;
    if (turn === opening) editor.value = { ...next, key: editor.value.key + 1, note };
  },
);
// The note of the second panel opened in the first trades places with the note there: never one note in both (#62).
const { beside, besidePanel } = useTabs();
const split = useSplit();
watch(
  () => route.params.id as string,
  (id, before) =>
    id === beside.value &&
    (besidePanel.value =
      before === 'new' ? withoutTab(besidePanel.value, id) : openTab(besidePanel.value, { id: before }, false)),
);

// The panel keeps the related notes and the graph from one note to the next, not for a note opened long after.
onUnmounted(() => clearNuxtState(['related-shown', 'graph-shown']));

function onCreated(id: string) {
  createdId = id;
  navigateTo(`/notes/${id}`, { replace: true });
}
</script>

<template>
  <div class="flex min-h-0 flex-1 max-lg:flex-col max-lg:overflow-y-auto">
    <!-- Below lg the two panels stack, half the height each; the page scrolls only when a half would get under 18rem. -->
    <!-- Split, the first panel is resized like the sidebar, its width kept by the layout's UDashboardGroup. Alone, it
         fills the page and hides its handle: the same panel either way, or opening a note beside would remount the editor. -->
    <UDashboardPanel
      id="note-main"
      resizable
      :min-size="20"
      :default-size="40"
      :ui="
        split.shown.value
          ? { root: 'min-h-0 border-0 max-lg:min-h-72 max-lg:flex-1', handle: 'border-x border-default' }
          : { root: 'min-h-0 flex-1 border-0 lg:w-auto', handle: 'hidden' }
      "
    >
      <NoteTabs />
      <NoteEditor
        :key="editor.key"
        :note-id="editor.noteId"
        :parent-id="editor.parentId"
        :start="editor.start"
        :note="editor.note"
        @created="onCreated"
      />
    </UDashboardPanel>
    <section
      v-if="split.shown.value"
      :aria-label="$t('note.beside')"
      class="flex min-w-0 flex-1 flex-col max-lg:min-h-72 max-lg:border-t max-lg:border-default"
    >
      <NoteEditor :key="beside" :note-id="beside" beside />
    </section>
  </div>
</template>
