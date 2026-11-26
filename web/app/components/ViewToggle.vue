<script setup lang="ts">
const { icons } = useIcons();

const view = useView();
const { on, available, active } = usePreview();
</script>

<template>
  <UTooltip :text="view === 'grid' ? $t('folder.listView') : $t('folder.gridView')">
    <UButton
      :icon="view === 'grid' ? icons.list : icons.grid"
      color="neutral"
      variant="ghost"
      :aria-label="view === 'grid' ? $t('folder.listView') : $t('folder.gridView')"
      @click="view = view === 'grid' ? 'list' : 'grid'"
    />
  </UTooltip>
  <!-- The lists in rows beside the note clicked, read only (#256). -->
  <UTooltip v-if="available" :text="$t('folder.preview')">
    <UButton
      :icon="icons.beside"
      :color="active ? 'primary' : 'neutral'"
      variant="ghost"
      :aria-label="$t('folder.preview')"
      :aria-pressed="active"
      @click="on = !on"
    />
  </UTooltip>
</template>
