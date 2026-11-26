<script setup lang="ts">
import { useResizable } from '@nuxt/ui/runtime/composables/useResizable.js';

// Around a list of notes (folder, tag, search): with the preview on, the list scrolls in its own pane and a plain click
// on a note shows it read only beside (#256), a double click opens it. Folders, ⌘-click, ⇧-click and the lasso stay
// the list's.
const { active, shown } = usePreview();
const { icons } = useIcons();
onUnmounted(() => (shown.value = undefined));

// The list's width, dragged on the line between both panes (double click: back to the default), kept in localStorage.
const {
  el: list,
  size,
  onMouseDown,
  onTouchStart,
  onDoubleClick,
} = useResizable('list-preview-width', {
  side: 'left',
  unit: 'rem',
  storage: 'local',
  minSize: 16,
  defaultSize: 24,
  maxSize: 48,
});

function noteOf(event: MouseEvent) {
  const target = event.target as HTMLElement;
  const row = target.closest<HTMLElement>('[data-id]');
  return row && target.closest('a')?.getAttribute('href')?.startsWith('/notes/') ? row.dataset.id : undefined;
}

function preview(event: MouseEvent) {
  if (!active.value || event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const id = noteOf(event);
  if (!id) return;
  event.preventDefault();
  event.stopPropagation(); // The link must not open.
  shown.value = id;
}

function open(event: MouseEvent) {
  const id = active.value && noteOf(event);
  if (id) return navigateTo(`/notes/${id}`);
}

// A [[link]] of the note shown is shown in its turn.
function follow(event: MouseEvent) {
  const id = (event.target as HTMLElement).closest<HTMLElement>('[data-note]')?.dataset.note;
  if (id) shown.value = id;
}
</script>

<template>
  <div class="flex flex-1" :class="{ 'min-h-0': active }">
    <!-- data-slot="body": the list scrolls in its own pane, where useVirtualRows looks for its scroll. -->
    <div
      ref="list"
      class="@container flex min-w-0 flex-col"
      :class="active ? 'min-h-0 shrink-0 overflow-y-auto' : 'flex-1'"
      :style="active ? { width: `${size}rem` } : undefined"
      :data-slot="active ? 'body' : undefined"
      @click.capture="preview"
      @dblclick="open"
    >
      <slot />
    </div>
    <UDashboardResizeHandle
      v-if="active"
      :aria-label="$t('folder.resizePreview')"
      class="w-px bg-(--ui-border) transition-colors hover:bg-primary"
      @mousedown="onMouseDown"
      @touchstart="onTouchStart"
      @dblclick="onDoubleClick"
    />
    <section
      v-if="active"
      :aria-label="$t('folder.preview')"
      class="flex min-h-0 min-w-0 flex-1 flex-col"
      @click="follow"
    >
      <NotePreview v-if="shown" :note-id="shown" />
      <UEmpty v-else :icon="icons.note" :title="$t('folder.previewEmpty')" class="m-auto" />
    </section>
  </div>
</template>
