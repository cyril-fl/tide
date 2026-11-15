<script setup lang="ts">
import type { Change, Choice, MergePart } from '~/utils/merge';

// After a 409 with changes in the same place (#78): the merged text, each conflict at its place with the text typed here,
// the other device's and a choice. Closes with the text chosen; the text typed here is already kept in the history.
// fork: the merge of a fork into its original, the fork's text against the original's; it can be cancelled (undefined).
const props = defineProps<{ parts: MergePart[]; fork?: boolean }>();
const emit = defineEmits<{ close: [text?: string] }>();

const phone = useMediaQuery('(max-width: 639px)');
const conflicts = props.parts.filter((part) => !('lines' in part));
const choices = ref<(Choice | undefined)[]>(conflicts.map(() => undefined));
const left = computed(() => choices.value.filter((choice) => !choice).length);
const options: { choice: Choice; label: string }[] = [
  { choice: 'mine', label: props.fork ? 'conflict.keepFork' : 'conflict.mine' },
  { choice: 'theirs', label: props.fork ? 'conflict.keepOriginal' : 'conflict.theirs' },
  { choice: 'both', label: 'conflict.both' },
];

// The index of each conflict among the parts, for its choice.
const indexOf = (part: MergePart) => conflicts.indexOf(part);
// Each conflict from the other side to this one: its lines removed shown on the other side, added on this one.
const changes = conflicts.map((part) =>
  'mine' in part ? lineChanges(part.theirs.join('\n'), part.mine.join('\n')) : [],
);
const sides = (part: MergePart) => {
  const all = changes[indexOf(part)] ?? [];
  return [
    {
      label: props.fork ? 'conflict.fork' : 'conflict.here',
      changes: all.filter((change) => change.kind !== 'removed'),
    },
    {
      label: props.fork ? 'conflict.original' : 'conflict.elsewhere',
      changes: all.filter((change) => change.kind !== 'added'),
    },
  ];
};
const tag = { same: 'span', removed: 'del', added: 'ins' } satisfies Record<Change['kind'], string>;
const color = { same: '', removed: 'bg-error/10 text-error', added: 'bg-success/10 text-success' };
const save = () => emit('close', mergedText(props.parts, choices.value as Choice[]));
const keepTheirs = () =>
  emit(
    'close',
    mergedText(
      props.parts,
      conflicts.map(() => 'theirs'),
    ),
  );
</script>

<template>
  <UModal
    :title="$t('conflict.title', { count: left })"
    :description="$t(fork ? 'conflict.forkDescription' : 'conflict.description')"
    :fullscreen="phone"
    :dismissible="false"
    :close="false"
    :ui="{ content: 'sm:max-w-5xl' }"
  >
    <template #body>
      <div class="flex flex-col gap-2 text-sm">
        <template v-for="(part, index) in parts" :key="index">
          <pre v-if="'lines' in part" class="font-sans whitespace-pre-wrap">{{ part.lines.join('\n') }}</pre>
          <section
            v-else
            class="flex flex-col gap-2 rounded-md border p-3"
            :class="choices[indexOf(part)] ? 'border-default' : 'border-warning'"
            :aria-label="$t('conflict.block', { number: indexOf(part) + 1 })"
          >
            <div class="grid gap-3 sm:grid-cols-2">
              <div v-for="side in sides(part)" :key="side.label" class="flex min-w-0 flex-col gap-1">
                <span class="text-xs text-muted">{{ $t(side.label) }}</span>
                <component
                  :is="tag[change.kind]"
                  v-for="(change, line) in side.changes"
                  :key="line"
                  class="block whitespace-pre-wrap no-underline"
                  :class="color[change.kind]"
                  >{{ change.lines.join('\n') }}</component
                >
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
              <UButton
                v-for="option in options"
                :key="option.choice"
                :label="$t(option.label)"
                size="xs"
                :color="choices[indexOf(part)] === option.choice ? 'primary' : 'neutral'"
                :variant="choices[indexOf(part)] === option.choice ? 'solid' : 'outline'"
                :aria-pressed="choices[indexOf(part)] === option.choice"
                @click="choices[indexOf(part)] = option.choice"
              />
            </div>
          </section>
        </template>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full flex-wrap justify-between gap-2">
        <UButton
          :label="$t(fork ? 'conflict.keepOriginalEverywhere' : 'conflict.keepTheirs')"
          color="neutral"
          variant="ghost"
          @click="keepTheirs"
        />
        <div class="flex gap-2">
          <UButton v-if="fork" :label="$t('common.cancel')" color="neutral" variant="outline" @click="emit('close')" />
          <UButton :label="$t('conflict.save')" :disabled="left > 0" @click="save" />
        </div>
      </div>
    </template>
  </UModal>
</template>
