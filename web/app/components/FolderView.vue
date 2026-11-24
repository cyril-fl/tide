<script setup lang="ts">
import type { Item } from '~/types/items';

const { icons } = useIcons();

const props = defineProps<{ folderId?: string; title: string; subtitle?: string }>();

const { createFolder, moveItem, removeItems, exportItems } = useItems();
const view = useView();
const { items, status, load } = useFolderPages(props.folderId);

// ⌘-click (Ctrl elsewhere) adds or takes out an item, ⇧-click selects a range from the last one; a plain click opens it.
const selection = useSelection();
const selectedIds = computed(() => new Set(selection.value));
const selected = computed(() => items.value.filter((item) => item && selectedIds.value.has(item.id)) as Item[]);
let anchor = -1;

function select(event: MouseEvent, index: number) {
  const item = items.value[index];
  if (!item || (!event.metaKey && !event.ctrlKey && !event.shiftKey)) return;
  event.preventDefault();
  event.stopPropagation(); // The link under it must not open.
  if (event.shiftKey && anchor >= 0) {
    // Only the items read so far: the range skips the pages not read yet.
    selection.value = selectRange(
      selection.value,
      items.value.map((item) => item?.id),
      anchor,
      index,
    );
    return;
  }
  selection.value = toggleSelected(selection.value, item.id);
  anchor = index;
}

onKeyStroke('Escape', () => (selection.value = []));

// ⌘A (Ctrl elsewhere) selects every item of the folder, reading the pages not read yet; again, it unselects them.
const { key } = useShortcuts();
defineShortcuts({
  [key('selectAll')]: async () => {
    if (items.value.length && selection.value.length === items.value.length) return (selection.value = []);
    await load(0, items.value.length - 1);
    selection.value = items.value.flatMap((item) => (item ? [item.id] : []));
  },
  // Not while typing in a field: defineShortcuts skips them unless asked.
  delete: () => selection.value.length && removeSelection(),
  backspace: () => selection.value.length && removeSelection(),
});

// A selected folder cannot go into itself: it is not offered.
const destinations = useDestinations(() => selection.value);

const moving = ref(false);
const folders = computed(() =>
  destinations.value.map((folder) => ({
    label: folder.label,
    icon: icons.folder,
    onSelect: async () => {
      moving.value = false;
      await moveItem(selection.value, folder.value === ROOT ? null : folder.value, props.folderId ?? null);
      selection.value = [];
    },
  })),
);

async function removeSelection() {
  await removeItems(selected.value);
  selection.value = [];
}

const panel = useTemplateRef('panel');
const isOver = useDropFolder(panel, () => props.folderId ?? null);

// A row is h-9, a tile has two text-xs lines under its square.
const list = useTemplateRef('list');
const lasso = useLasso(list);
const rows = useVirtualRows(
  list,
  () => items.value.length,
  () => view.value === 'grid',
  { row: 36, caption: 32 },
);
const visible = computed(() => ({ ...rows.value, items: items.value.slice(rows.value.start, rows.value.end) }));
// The rows in sight that are not read yet come by pages.
watchEffect(() => load(visible.value.start, visible.value.start + visible.value.items.length - 1));
</script>

<template>
  <div
    ref="panel"
    class="flex min-h-full flex-col p-8 transition-shadow duration-150"
    :class="{ 'ring-2 ring-inset ring-primary/40': isOver }"
  >
    <UPageHeader :title="title">
      <template #links>
        <SortMenu />
        <ViewToggle />
        <UTooltip :text="$t('common.newFolder')">
          <UButton
            :icon="icons.folderAdd"
            color="neutral"
            variant="ghost"
            :aria-label="$t('common.newFolder')"
            @click="createFolder(folderId)"
          />
        </UTooltip>
        <UTooltip :text="$t('common.newNote')">
          <UButton
            :icon="icons.noteAdd"
            color="neutral"
            variant="ghost"
            :aria-label="$t('common.newNote')"
            :to="newNotePath(folderId)"
          />
        </UTooltip>
      </template>
    </UPageHeader>
    <div v-if="lasso" class="pointer-events-none fixed z-50 border border-primary/60 bg-primary/10" :style="lasso" />
    <ul
      v-if="items.length"
      ref="list"
      :class="{ mosaic: view === 'grid' }"
      :style="{ paddingTop: `${visible.before}px`, paddingBottom: `${visible.after}px` }"
    >
      <li
        v-for="(item, index) in visible.items"
        :key="item?.id ?? visible.start + index"
        :data-id="item?.id"
        @click.capture="select($event, visible.start + index)"
      >
        <ItemRow v-if="item" :item="item" :grid="view === 'grid'" :selected="selectedIds.has(item.id)" />
        <!-- A tile is a square, then gap-2 and its two lines of text. -->
        <USkeleton v-else :class="view === 'grid' ? 'mb-10 aspect-square' : 'h-9'" />
      </li>
    </ul>
    <UEmpty
      v-else-if="status === 'success'"
      :icon="icons.folderEmpty"
      :title="$t('folder.empty')"
      :description="$t('folder.emptyDescription')"
      :actions="[
        { label: $t('common.newNote'), icon: icons.noteAdd, to: newNotePath(folderId) },
        {
          label: $t('common.newFolder'),
          icon: icons.folderAdd,
          color: 'neutral',
          variant: 'subtle',
          onClick: () => createFolder(folderId),
        },
      ]"
    />
    <!-- Like a note's footer, the selection's actions with the navigation. The subtitle is the folder's own path: the breadcrumb leads to its parents. -->
    <footer
      v-if="subtitle || selected.length"
      class="sticky bottom-0 z-10 -mx-8 -mb-8 mt-auto border-t border-default bg-default px-8"
    >
      <div class="flex h-9 items-center gap-2 text-xs text-muted">
        <PathNav v-if="subtitle" :path="subtitle.slice(0, subtitle.lastIndexOf('/'))" />
        <template v-if="selected.length">
          <span class="ml-auto shrink-0">{{ $t('folder.selected', selected.length) }}</span>
          <UPopover v-model:open="moving">
            <UTooltip :text="$t('folder.moveTo')">
              <UButton :icon="icons.move" color="neutral" variant="ghost" size="xs" :aria-label="$t('folder.moveTo')" />
            </UTooltip>
            <template #content>
              <UCommandPalette
                :groups="[{ id: 'folders', items: folders }]"
                :placeholder="$t('folder.moveToPlaceholder')"
                class="max-h-72 w-72"
              />
            </template>
          </UPopover>
          <UTooltip :text="$t('settings.export')">
            <UButton
              :icon="icons.export"
              color="neutral"
              variant="ghost"
              size="xs"
              :aria-label="$t('settings.export')"
              @click="exportItems(selection)"
            />
          </UTooltip>
          <UTooltip :text="$t('common.delete')">
            <UButton
              :icon="icons.delete"
              color="error"
              variant="ghost"
              size="xs"
              :aria-label="$t('common.delete')"
              @click="removeSelection"
            />
          </UTooltip>
          <UTooltip :text="$t('folder.clearSelection')" :kbds="['esc']">
            <UButton
              :icon="icons.close"
              color="neutral"
              variant="ghost"
              size="xs"
              :aria-label="$t('folder.clearSelection')"
              @click="selection = []"
            />
          </UTooltip>
        </template>
      </div>
    </footer>
  </div>
</template>
