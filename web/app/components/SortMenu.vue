<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui';

const { icons } = useIcons();
const { t } = useI18n();

// The sort is a list of keys, the first deciding first, "-" before one to reverse; "" is neutral (by name).
const KEYS = ['title', 'created', 'updated', 'pinned'] as const;
const sort = useSort();
const keys = computed(() => (sort.value ? sort.value.split(',') : []));

// Each click on a choice: ascending (added last), then descending (same place), then off.
function cycle(key: string) {
  const list = keys.value;
  const index = list.findIndex((k) => k.replace(/^-/, '') === key);
  if (index < 0) sort.value = [...list, key].join(',');
  else if (!list[index]!.startsWith('-')) sort.value = list.with(index, `-${key}`).join(',');
  else sort.value = list.toSpliced(index, 1).join(',');
}

function label(key: (typeof KEYS)[number], reversed: boolean | undefined) {
  if (key === 'pinned' && reversed !== undefined)
    return t(reversed ? 'folder.sortPinnedLast' : 'folder.sortPinnedFirst');
  return t(`folder.sort${key[0]!.toUpperCase()}${key.slice(1)}`);
}

const menu = computed<DropdownMenuItem[][]>(() => [
  [
    { type: 'label', label: t('folder.sort') },
    ...KEYS.map((key) => {
      const index = keys.value.findIndex((k) => k.replace(/^-/, '') === key);
      const reversed = index < 0 ? undefined : keys.value[index]!.startsWith('-');
      return {
        label: label(key, reversed),
        icon: reversed ? icons.descending : icons.ascending,
        color: reversed !== undefined ? ('primary' as const) : undefined,
        // The icon of an idle choice keeps its room: the labels stay aligned.
        ui: { itemLeadingIcon: reversed !== undefined ? 'text-primary' : 'invisible' },
        // Its rank when several choices are on: the first decides first.
        kbds: index >= 0 && keys.value.length > 1 ? [String(index + 1)] : undefined,
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
      disabled: !sort.value,
      onSelect: () => (sort.value = ''),
    },
  ],
]);
</script>

<template>
  <UDropdownMenu :items="menu" :content="{ align: 'end' }">
    <UTooltip :text="$t('folder.sort')">
      <UButton
        :icon="icons.sort"
        :color="sort ? 'primary' : 'neutral'"
        variant="ghost"
        :aria-label="$t('folder.sort')"
      />
    </UTooltip>
  </UDropdownMenu>
</template>
