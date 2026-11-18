<script setup lang="ts">
import type { ReplaceResult } from '~/types/items';

// Replace in every note like VS Code's search: the occurrences listed by note, replaced one by one, by note or all.
const props = defineProps<{ find?: string }>();
const emit = defineEmits<{ close: [] }>();

const { t } = useI18n();
const { icons } = useIcons();
const { confirm } = useDialogs();
const api = useApi();
const toast = useToast();

const find = ref(props.find ?? '');
// Empty: the text is removed.
const replace = ref('');
const search = refDebounced(find, 250);
const busy = ref(false);

const replaceNotes = (body: { dry_run?: boolean; notes?: string[]; occurrence?: number }) =>
  api<ReplaceResult>('/api/replace', { method: 'POST', body: { find: search.value, replace: replace.value, ...body } });

const { data: result, refresh } = useLazyAsyncData(
  'replace-matches',
  () => (search.value.trim() ? replaceNotes({ dry_run: true }) : Promise.resolve(undefined)),
  { watch: [search] },
);
const listed = computed(() => (result.value?.matches ?? []).reduce((sum, match) => sum + match.occurrences.length, 0));
const locked = computed(() =>
  result.value?.locked ? t('search.replace.locked', { n: result.value.locked }, result.value.locked) : '',
);

async function apply(body: { notes?: string[]; occurrence?: number }) {
  busy.value = true;
  try {
    const done = await replaceNotes(body);
    if (!body.occurrence)
      toast.add({ title: t('search.replace.done', { n: done.notes }, done.notes), color: 'success' });
    // The open note and the lists show the new text.
    await refreshNuxtData();
    await refresh();
  } finally {
    busy.value = false;
  }
}

async function applyAll() {
  const count = result.value;
  if (!count?.notes) return;
  const confirmed = await confirm({
    title: t('search.replace.confirm', { find: search.value, replace: replace.value }),
    description: [
      t('search.replace.notes', { n: count.notes, occurrences: count.occurrences }, count.notes),
      locked.value,
    ]
      .filter(Boolean)
      .join(' '),
    action: t('search.replace.submit'),
  });
  if (confirmed) await apply({});
}
</script>

<template>
  <UModal
    :title="$t('search.replace.action')"
    :ui="{ content: 'max-w-2xl', body: 'flex flex-col gap-3' }"
    @update:open="emit('close')"
  >
    <template #body>
      <div class="flex flex-col gap-2">
        <UInput
          v-model="find"
          autofocus
          class="w-full"
          :placeholder="$t('search.replace.find')"
          :aria-label="$t('search.replace.find')"
        />
        <UInput
          v-model="replace"
          class="w-full"
          :placeholder="$t('search.replace.with')"
          :aria-label="$t('search.replace.with')"
          @keydown.enter.prevent="applyAll"
        />
      </div>
      <p v-if="result" class="text-sm text-muted">
        {{
          result.notes
            ? $t('search.replace.notes', { n: result.notes, occurrences: result.occurrences }, result.notes)
            : $t('search.replace.none', { find: search })
        }}
        {{ locked }}
        <template v-if="listed < result.occurrences">{{ $t('search.replace.more', { n: listed }) }}</template>
      </p>
      <ul v-if="result?.matches?.length" class="-mx-2 max-h-[55vh] overflow-y-auto">
        <li v-for="match in result.matches" :key="match.id" class="mb-2">
          <div class="flex items-center gap-2 px-2 py-1">
            <UIcon :name="icons.note" class="size-4 shrink-0 text-muted" />
            <NuxtLink :to="`/notes/${match.id}`" class="min-w-0 truncate text-sm font-medium" @click="emit('close')">
              {{ match.path }}
            </NuxtLink>
            <UBadge :label="String(match.occurrences.length)" color="neutral" variant="soft" size="sm" />
            <UButton
              :icon="icons.replace"
              size="xs"
              color="neutral"
              variant="ghost"
              class="ml-auto"
              :disabled="busy"
              :aria-label="$t('search.replace.inNote')"
              :title="$t('search.replace.inNote')"
              @click="apply({ notes: [match.id] })"
            />
          </div>
          <ul>
            <li
              v-for="(line, index) in match.occurrences"
              :key="index"
              class="flex items-start gap-2 rounded-md py-0.5 pr-2 pl-2 sm:pl-8 text-sm hover:bg-elevated"
            >
              <span class="w-8 shrink-0 py-1 text-right text-xs text-dimmed tabular-nums">{{ line.line }}</span>
              <span class="min-w-0 flex-1 py-1 font-mono break-words text-xs">
                <span class="text-muted">{{ line.before }}</span>
                <span :class="replace ? 'bg-error/15 text-error line-through' : 'bg-primary/20'">{{ search }}</span>
                <span v-if="replace" class="bg-success/15 text-success">{{ replace }}</span>
                <span class="text-muted">{{ line.after }}</span>
              </span>
              <UButton
                :icon="icons.replace"
                size="xs"
                color="neutral"
                variant="ghost"
                :disabled="busy"
                :aria-label="$t('search.replace.one')"
                :title="$t('search.replace.one')"
                @click="apply({ notes: [match.id], occurrence: index + 1 })"
              />
            </li>
          </ul>
        </li>
      </ul>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton :label="$t('common.close')" color="neutral" variant="ghost" @click="emit('close')" />
        <UButton
          :label="$t('search.replace.submit')"
          :icon="icons.replace"
          :disabled="!result?.notes || busy"
          :loading="busy"
          @click="applyAll"
        />
      </div>
    </template>
  </UModal>
</template>
