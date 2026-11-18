<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui';

const { icons } = useIcons();
const { t } = useI18n();

const DEFAULT = 'title';
const sort = useSort();
const by = computed(() => sort.value.replace(/^-/, ''));
const reversed = computed(() => sort.value.startsWith('-'));

// Each click on a choice: ascending, then descending, then back to the default order (the name, ascending).
function cycle(key: string) {
  if (by.value !== key) sort.value = key;
  else if (!reversed.value) sort.value = `-${key}`;
  else sort.value = DEFAULT;
}

const menu = computed<DropdownMenuItem[][]>(() => [
  [
    { type: 'label', label: t('folder.sort') },
    ...(['title', 'created', 'updated'] as const).map((key) => {
      const active = by.value === key;
      return {
        label: t(`folder.sort${key[0]!.toUpperCase()}${key.slice(1)}`),
        icon: active && reversed.value ? icons.descending : icons.ascending,
        color: active ? ('primary' as const) : undefined,
        // The icon of an idle choice keeps its room: the labels stay aligned.
        ui: { itemLeadingIcon: active ? 'text-primary' : 'invisible' },
        onSelect: (event: Event) => {
          event.preventDefault(); // The menu stays open for the next click.
          cycle(key);
        },
      };
    }),
  ],
  [
    {
      label: t('folder.sortReset'),
      icon: icons.close,
      disabled: sort.value === DEFAULT,
      onSelect: () => (sort.value = DEFAULT),
    },
  ],
]);
</script>

<template>
  <UDropdownMenu :items="menu" :content="{ align: 'end' }">
    <UTooltip :text="$t('folder.sort')">
      <UButton :icon="icons.sort" color="neutral" variant="ghost" :aria-label="$t('folder.sort')" />
    </UTooltip>
  </UDropdownMenu>
</template>
