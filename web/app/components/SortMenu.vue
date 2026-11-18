<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui';

const { icons } = useIcons();
const { t } = useI18n();

const sort = useSort();
const by = computed(() => sort.value.replace(/^-/, ''));
const reversed = computed(() => sort.value.startsWith('-'));

// Like a Finder: the name A to Z, the dates newest first, each choice keeping its direction once picked.
const menu = computed<DropdownMenuItem[][]>(() => [
  [
    { type: 'label', label: t('folder.sort') },
    ...(['title', 'created', 'updated'] as const).map((key) => ({
      label: t(`folder.sort${key[0]!.toUpperCase()}${key.slice(1)}`),
      type: 'checkbox' as const,
      checked: by.value === key,
      onSelect: () => (sort.value = key === 'title' ? key : `-${key}`),
    })),
  ],
  [false, true].map((descending) => ({
    label: t(descending ? 'folder.descending' : 'folder.ascending'),
    icon: descending ? icons.descending : icons.ascending,
    type: 'checkbox' as const,
    checked: reversed.value === descending,
    onSelect: () => (sort.value = `${descending ? '-' : ''}${by.value}`),
  })),
]);
</script>

<template>
  <UDropdownMenu :items="menu" :content="{ align: 'end' }">
    <UTooltip :text="$t('folder.sort')">
      <UButton :icon="icons.sort" color="neutral" variant="ghost" :aria-label="$t('folder.sort')" />
    </UTooltip>
  </UDropdownMenu>
</template>
