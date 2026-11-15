<script setup lang="ts">
import { parseDate, type DateValue } from '@internationalized/date';

// A range of days typed or picked in a calendar, each bound a "YYYY-MM-DD" day, "" when left open.
const from = defineModel<string>('from', { required: true });
const to = defineModel<string>('to', { required: true });
defineProps<{ label: string }>();

const { icons } = useIcons();
const input = useTemplateRef('input');
const range = computed<{ start: DateValue | undefined; end: DateValue | undefined }>({
  get: () => ({
    start: from.value ? parseDate(from.value) : undefined,
    end: to.value ? parseDate(to.value) : undefined,
  }),
  set: (value) => {
    from.value = value?.start?.toString().slice(0, 10) ?? '';
    to.value = value?.end?.toString().slice(0, 10) ?? '';
  },
});
</script>

<template>
  <UInputDate ref="input" v-model="range" range class="w-full" :aria-label="label">
    <template #trailing>
      <UPopover :reference="input?.inputsRef[0]?.$el">
        <UButton :icon="icons.calendar" color="neutral" variant="link" size="sm" class="px-0" :aria-label="label" />
        <template #content>
          <UCalendar v-model="range" range class="p-2" />
        </template>
      </UPopover>
    </template>
  </UInputDate>
</template>
