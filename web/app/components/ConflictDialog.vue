<script setup lang="ts">
import type { Choice, MergePart } from '~/utils/merge';

// After a 409 with changes in the same place (#78): the merged text, each conflict at its place with the text typed here,
// the other device's and the result between them, built from a choice or line by line with arrows, like a merge tool:
// an arrow points to the result to take its line, back once taken.
// Closes with the text chosen; the text typed here is already kept in the history.
// fork: the merge of a fork into its original or its rebase, the fork's text against the original's; it can be cancelled (undefined).
const props = defineProps<{ parts: MergePart[]; fork?: boolean }>();
const emit = defineEmits<{ close: [text?: string] }>();

const { icons } = useIcons();
const phone = useMediaQuery('(max-width: 639px)');
const conflicts = props.parts.filter((part): part is Exclude<MergePart, { lines: string[] }> => !('lines' in part));
// Each conflict's lines from the other side to this one, the lines kept and whether it was resolved yet.
const rows = conflicts.map((part) => conflictRows(part));
const picks = ref(rows.map((lines) => lines.map((row) => row.kind === 'same')));
const touched = ref(conflicts.map(() => false));
const left = computed(() => touched.value.filter((done) => !done).length);
const options: { choice: Choice; label: string }[] = [
  { choice: 'mine', label: props.fork ? 'conflict.keepFork' : 'conflict.mine' },
  { choice: 'theirs', label: props.fork ? 'conflict.keepOriginal' : 'conflict.theirs' },
  { choice: 'both', label: 'conflict.both' },
];

// The index of each conflict among the parts, for its lines.
const indexOf = (part: MergePart) => conflicts.findIndex((conflict) => conflict === part);
const pick = (conflict: number, choice: Choice) => {
  picks.value[conflict] = choose(rows[conflict]!, choice);
  touched.value[conflict] = true;
};
const toggle = (conflict: number, line: number) => {
  picks.value[conflict]![line] = !picks.value[conflict]![line];
  touched.value[conflict] = true;
};
const chosen = (conflict: number, choice: Choice) =>
  touched.value[conflict] &&
  choose(rows[conflict]!, choice).every((kept, line) => kept === picks.value[conflict]![line]);
const result = (conflict: number) => pickedLines(rows[conflict]!, picks.value[conflict]!);
const save = () =>
  emit(
    'close',
    mergedText(
      props.parts,
      conflicts.map((_, conflict) => result(conflict)),
    ),
  );
const keepTheirs = () =>
  emit(
    'close',
    mergedText(
      props.parts,
      rows.map((lines) => pickedLines(lines, choose(lines, 'theirs'))),
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
    :ui="{ content: 'sm:max-w-6xl' }"
  >
    <template #body>
      <div class="flex flex-col gap-2 text-sm">
        <template v-for="(part, index) in parts" :key="index">
          <pre v-if="'lines' in part" class="font-sans whitespace-pre-wrap">{{ part.lines.join('\n') }}</pre>
          <section
            v-else
            class="flex flex-col gap-2 rounded-md border p-3"
            :class="touched[indexOf(part)] ? 'border-default' : 'border-warning'"
            :aria-label="$t('conflict.block', { number: indexOf(part) + 1 })"
          >
            <div class="grid gap-3 sm:grid-cols-3">
              <div class="flex min-w-0 flex-col">
                <span class="mb-1 text-xs text-muted">{{ $t(fork ? 'conflict.fork' : 'conflict.here') }}</span>
                <template v-for="(row, line) in rows[indexOf(part)]" :key="line">
                  <span v-if="row.kind === 'same'" class="whitespace-pre-wrap">{{ row.line || ' ' }}</span>
                  <div v-else-if="row.kind === 'added'" class="flex items-start gap-1 bg-success/10 text-success">
                    <ins class="min-w-0 flex-1 whitespace-pre-wrap no-underline">{{ row.line || ' ' }}</ins>
                    <UButton
                      :icon="picks[indexOf(part)]![line] ? icons.back : icons.forward"
                      size="xs"
                      color="success"
                      variant="ghost"
                      :aria-label="$t('conflict.take')"
                      :aria-pressed="picks[indexOf(part)]![line]"
                      @click="toggle(indexOf(part), line)"
                    />
                  </div>
                </template>
              </div>
              <div class="flex min-w-0 flex-col">
                <span class="mb-1 text-xs text-muted">{{ $t('conflict.result') }}</span>
                <pre
                  class="min-h-6 flex-1 rounded-sm bg-elevated px-1 font-sans whitespace-pre-wrap"
                  :aria-label="$t('conflict.result')"
                  >{{ result(indexOf(part)).join('\n') }}</pre>
              </div>
              <div class="flex min-w-0 flex-col">
                <span class="mb-1 text-xs text-muted">{{ $t(fork ? 'conflict.original' : 'conflict.elsewhere') }}</span>
                <template v-for="(row, line) in rows[indexOf(part)]" :key="line">
                  <span v-if="row.kind === 'same'" class="whitespace-pre-wrap">{{ row.line || ' ' }}</span>
                  <div v-else-if="row.kind === 'removed'" class="flex items-start gap-1 bg-error/10 text-error">
                    <UButton
                      :icon="picks[indexOf(part)]![line] ? icons.forward : icons.back"
                      size="xs"
                      color="error"
                      variant="ghost"
                      :aria-label="$t('conflict.take')"
                      :aria-pressed="picks[indexOf(part)]![line]"
                      @click="toggle(indexOf(part), line)"
                    />
                    <del class="min-w-0 flex-1 whitespace-pre-wrap no-underline">{{ row.line || ' ' }}</del>
                  </div>
                </template>
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
              <UButton
                v-for="option in options"
                :key="option.choice"
                :label="$t(option.label)"
                size="xs"
                :color="chosen(indexOf(part), option.choice) ? 'primary' : 'neutral'"
                :variant="chosen(indexOf(part), option.choice) ? 'solid' : 'outline'"
                :aria-pressed="chosen(indexOf(part), option.choice)"
                @click="pick(indexOf(part), option.choice)"
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
