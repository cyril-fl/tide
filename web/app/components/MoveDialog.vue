<script setup lang="ts">
// The folder an item goes to, filtered as typed: ROOT for the root. The item itself is not offered.
const props = defineProps<{ exclude: string[] }>();
const emit = defineEmits<{ close: [folder?: string] }>();

const { icons } = useIcons();
const destinations = useDestinations(() => props.exclude);
const folders = computed(() =>
  destinations.value.map((folder) => ({
    label: folder.label,
    icon: icons.folder,
    onSelect: () => emit('close', folder.value),
  })),
);
</script>

<template>
  <UModal :title="$t('folder.moveTo')" :ui="{ content: 'max-w-sm' }" @update:open="emit('close')">
    <template #content>
      <UCommandPalette
        :groups="[{ id: 'folders', items: folders }]"
        :placeholder="$t('folder.moveToPlaceholder')"
        class="max-h-96"
      />
    </template>
  </UModal>
</template>
