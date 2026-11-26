<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui';

// A column header of a list: the label opens the column's filter when it has a menu, the arrow after it sorts.
const props = defineProps<{
  label: string;
  sorted: false | 'asc' | 'desc';
  active?: boolean;
  menu?: DropdownMenuItem[][];
}>();
const emit = defineEmits<{ sort: [] }>();
const { t } = useI18n();
const { icons } = useIcons();
</script>

<template>
  <div class="flex min-w-0 items-center gap-0.5">
    <UDropdownMenu v-if="menu" :items="menu" :content="{ align: 'start' }">
      <UButton
        :label="label"
        :color="active ? 'primary' : 'neutral'"
        :variant="active ? 'soft' : 'ghost'"
        :trailing-icon="icons.chevronDown"
        size="xs"
      />
    </UDropdownMenu>
    <span v-else class="truncate px-2 font-medium">{{ label }}</span>
    <UButton
      :color="sorted ? 'primary' : 'neutral'"
      variant="ghost"
      size="xs"
      :class="!sorted && 'text-dimmed'"
      :icon="sorted === 'asc' ? icons.ascending : sorted === 'desc' ? icons.descending : icons.sort"
      :aria-label="`${t('folder.sortBy')} ${props.label}`"
      @click="emit('sort')"
    />
  </div>
</template>
