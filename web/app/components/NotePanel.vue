<script setup lang="ts">
import type { ItemDetail } from '~/types/items';

// One panel of the editor (#253): its tabs, the editor of the note it shows, and while a tab is dragged, its edges to
// drop it on for a new panel there.
const props = defineProps<{ panel: string; index: number; stacked: boolean }>();

const route = useRoute();
const api = useApi();
const { data: me } = useMe();
const tabs = useTabs();
const split = useSplit();
const own = computed(() => tabs.layout.value.panels.find((panel) => panel.id === props.panel));
const active = computed(() => own.value?.active ?? '');
const isCurrent = computed(() => tabs.layout.value.current === props.panel);

// A new note takes its parent, its tags or its snippet from the address, only in the current panel.
function editorFor() {
  const id = active.value;
  if (id !== 'new') return { noteId: id || undefined };
  // TODO: a reload of /notes/new?snippet= before me is loaded starts empty, wait for me if that bites.
  const tags = typeof route.query.tags === 'string' && route.query.tags ? route.query.tags.split(',') : [];
  const start =
    me.value?.snippets[Number(route.query.snippet)]?.content ??
    // An empty first line above the tags takes the cursor and the title.
    (tags.length ? `&nbsp;\n\n${tags.map((tag) => `#${tag}`).join(' ')}` : undefined);
  return { noteId: undefined, parentId: route.query.parent as string | undefined, start };
}

const editor = ref<ReturnType<typeof editorFor> & { key: number; note?: ItemDetail }>({ key: 0, ...editorFor() });
let createdId: string | undefined;
let opening = 0;

// The next note is read before its editor replaces the one shown, or the panel blinks empty while it loads (#154).
// Not read, it loads itself and says why. Only the last note asked for opens. Another new note asked for (its address
// changed) gets a new editor too.
watch(
  () => [active.value, active.value === 'new' && isCurrent.value ? route.fullPath : ''],
  async ([id]) => {
    if (id === createdId) return void (createdId = undefined);
    const turn = ++opening;
    const next = editorFor();
    const note = next.noteId
      ? await api<ItemDetail>(`/api/notes/${next.noteId}`, { quiet: true }).catch(() => undefined)
      : undefined;
    if (turn === opening) editor.value = { ...next, key: editor.value.key + 1, note };
  },
);

// The new note keeps its editor, and the cursor in it, while its tab and the address take its id.
function onCreated(id: string) {
  createdId = id;
  tabs.created(id);
  if (isCurrent.value) navigateTo(`/notes/${id}`, { replace: true });
}

// A tab dropped on an edge opens a new panel on that side, while the screen holds one more.
const edges = computed(() => !!tabs.dragged.value && split.panels.value.length < split.max.value);
function dropAt(after: boolean) {
  const from = tabs.dragged.value;
  tabs.dragged.value = undefined;
  if (from) tabs.openAt(props.index + (after ? 1 : 0), from.id);
}
</script>

<template>
  <section
    :aria-label="$t('note.panel', { n: index + 1 })"
    class="flex h-full min-h-0 min-w-0 flex-col"
    @focusin="tabs.focus(panel)"
  >
    <NoteTabs :panel="panel" />
    <!-- The edges sit under the bar: a tab dropped on the bar joins this panel. -->
    <div class="relative flex min-h-0 flex-1 flex-col">
      <NoteEditor
        v-if="active"
        :key="editor.key"
        :note-id="editor.noteId"
        :parent-id="editor.parentId"
        :start="editor.start"
        :note="editor.note"
        :panel-id="panel"
        :current="isCurrent"
        @created="onCreated"
      />
      <template v-if="edges">
        <div
          v-for="after in [false, true]"
          :key="String(after)"
          :data-edge="after ? 'after' : 'before'"
          class="absolute z-20 bg-primary/10 ring-2 ring-primary/40 ring-inset"
          :class="
            stacked
              ? ['inset-x-0 h-12', after ? 'bottom-0' : 'top-0']
              : ['inset-y-0 w-12', after ? 'right-0' : 'left-0']
          "
          @dragover.prevent
          @drop.prevent.stop="dropAt(after)"
        />
      </template>
    </div>
  </section>
</template>
