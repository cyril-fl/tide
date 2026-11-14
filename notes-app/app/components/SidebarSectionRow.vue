<script setup lang="ts">
const { icons } = useIcons();
const props = defineProps<{ section: SidebarSection }>();
const { has } = usePro();
const { toggle, move } = useSidebarSections();

// Reordered from the Pro mode on: below it, the row is not draggable.
const row = useTemplateRef('row');
const draggable = computed(() => (has('sidebarOrder') ? row.value : null));
useReorderItem(draggable, () => props.section.id);
const isOver = useDropReorder(draggable, () => props.section.id, move);
</script>

<template>
  <li ref="row" class="flex items-center gap-4 py-2" :class="{ 'row-over': isOver }">
    <UIcon
      v-if="has('sidebarOrder')"
      :name="icons.reorder"
      :aria-label="$t('settings.sidebar.reorder')"
      class="size-4 shrink-0 cursor-grab text-muted"
    />
    <span class="min-w-0 flex-1 text-sm text-muted">{{ $t(`settings.sidebar.${section.id}`) }}</span>
    <USwitch
      :model-value="section.shown"
      :disabled="section.id === 'folders' && !has('hideFolders')"
      :aria-label="$t(`settings.sidebar.${section.id}`)"
      @update:model-value="toggle(section.id, $event)"
    />
  </li>
</template>
