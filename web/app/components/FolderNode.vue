<script setup lang="ts">
import type { Item } from '~/types/items';

const { icons } = useIcons();

const props = defineProps<{ folder: Item }>();

const route = useRoute();
const { itemMenu } = useItems();

const open = useState(`open:${props.folder.id}`, () => false);

// The whole node drags, so the check of useDropFolder sees the subfolders inside it.
const node = useTemplateRef('node');
const row = useTemplateRef('row');
useDragItem(node, () => props.folder);
const isOver = useDropFolder(row, () => props.folder.id);

const { start, stop } = useTimeoutFn(() => (open.value = true), 600, { immediate: false });
watch(isOver, (over) => (over ? start() : stop()));
</script>

<template>
  <div ref="node">
    <UContextMenu :items="itemMenu(folder)">
      <div ref="row" class="row" :class="{ 'row-active': route.params.id === folder.id, 'row-over': isOver }">
        <NuxtLink
          :to="`/folders/${folder.id}`"
          draggable="false"
          class="flex min-w-0 flex-1 items-center gap-2 self-stretch"
        >
          <UIcon :name="icons.folder" class="size-4 shrink-0" :class="isOver ? 'text-inverted' : 'text-primary'" />
          <span class="truncate">{{ folder.title }}</span>
        </NuxtLink>
        <UButton
          v-if="folder.has_folders"
          :icon="icons.chevronRight"
          color="neutral"
          variant="link"
          class="size-5 shrink-0 justify-center p-0"
          :ui="{ leadingIcon: `size-3.5 transition-transform duration-150 ${open ? 'rotate-90' : ''}` }"
          :aria-label="open ? $t('items.collapse') : $t('items.expand')"
          :aria-expanded="open"
          @click="open = !open"
        />
      </div>
    </UContextMenu>
    <FolderTree v-if="open" :parent-id="folder.id" />
  </div>
</template>
