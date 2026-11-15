<script setup lang="ts">
import type { SavedSearch } from '~/types/items';

const props = defineProps<{ title: string; value?: SavedSearch }>();
const emit = defineEmits<{ close: [value?: SavedSearch] }>();

const { t } = useI18n();
const { icons } = useIcons();
const api = useApi();
const { data: tags } = useTags();
const destinations = useDestinations();
const folderOptions = computed(() => destinations.value.filter((d) => d.value !== ROOT));
const tagOptions = computed(() => tags.value.map((tag) => ({ label: `#${tag.value}`, value: tag.id })));
const tagModeOptions = [
  { label: t('search.saved.tagModeAny'), value: 'any' },
  { label: t('search.saved.tagModeAll'), value: 'all' },
];
const folderModeOptions = [
  { label: t('search.saved.folderModeInclude'), value: 'include' },
  { label: t('search.saved.folderModeExclude'), value: 'exclude' },
];

// Date bounds are shown as a plain day (<input type="date">), sent as a day boundary in UTC.
function toDay(iso?: string) {
  return iso ? iso.slice(0, 10) : '';
}
function fromDay(day: string, end: boolean) {
  return day ? `${day}T${end ? '23:59:59.999Z' : '00:00:00.000Z'}` : undefined;
}

const name = ref(props.value?.name ?? '');
const tagIds = ref(props.value?.tag_ids ?? []);
const tagMode = ref(props.value?.tag_mode ?? 'any');
const folderIds = ref(props.value?.folder_ids ?? []);
const folderMode = ref(props.value?.folder_mode ?? 'include');
const createdFrom = ref(toDay(props.value?.created_from));
const createdTo = ref(toDay(props.value?.created_to));
const editedFrom = ref(toDay(props.value?.edited_from));
const editedTo = ref(toDay(props.value?.edited_to));
const text = ref(props.value?.text ?? '');

// Only the criteria in use are shown: those the search has, then those added from the menu.
type Criterion = 'text' | 'tags' | 'folders' | 'created' | 'edited';
const shown = ref<Criterion[]>(
  (
    [
      ['text', text.value],
      ['tags', tagIds.value.length],
      ['folders', folderIds.value.length],
      ['created', createdFrom.value || createdTo.value],
      ['edited', editedFrom.value || editedTo.value],
    ] as const
  )
    .filter(([, used]) => used)
    .map(([criterion]) => criterion),
);
const labels: Record<Criterion, string> = {
  text: t('search.saved.text'),
  tags: t('common.tags'),
  folders: t('search.saved.folders'),
  created: t('search.saved.created'),
  edited: t('search.saved.edited'),
};
const addMenu = computed(() =>
  (Object.keys(labels) as Criterion[])
    .filter((criterion) => !shown.value.includes(criterion))
    .map((criterion) => ({ label: labels[criterion], onSelect: () => shown.value.push(criterion) })),
);

function remove(criterion: Criterion) {
  shown.value = shown.value.filter((c) => c !== criterion);
  if (criterion === 'text') text.value = '';
  if (criterion === 'tags') tagIds.value = [];
  if (criterion === 'folders') folderIds.value = [];
  if (criterion === 'created') createdFrom.value = createdTo.value = '';
  if (criterion === 'edited') editedFrom.value = editedTo.value = '';
}

const criteria = computed<SavedSearch>(() => ({
  id: props.value?.id ?? '',
  name: name.value.trim(),
  tag_ids: tagIds.value,
  tag_mode: tagMode.value,
  folder_ids: folderIds.value,
  folder_mode: folderMode.value,
  created_from: fromDay(createdFrom.value, false),
  created_to: fromDay(createdTo.value, true),
  edited_from: fromDay(editedFrom.value, false),
  edited_to: fromDay(editedTo.value, true),
  text: text.value.trim(),
}));

// How many notes the criteria match as they are now, before saving them. The name is left out: typing it runs nothing.
const previewed = refDebounced(
  computed(() => JSON.stringify({ ...criteria.value, name: '' })),
  300,
);
const total = ref(0);
const { status } = useLazyAsyncData(
  'saved-search-preview',
  () =>
    api('/api/searches', {
      method: 'POST',
      body: previewed.value,
      headers: { 'Content-Type': 'application/json' },
      query: { limit: 1 },
      quiet: true,
      onResponse: ({ response }) => void (total.value = Number(response.headers.get('X-Total-Count'))),
    }),
  { watch: [previewed] },
);

function submit() {
  if (!name.value.trim()) return;
  emit('close', criteria.value);
}
</script>

<template>
  <UModal :title="title" :close="false" :ui="{ content: 'max-w-xl' }" @update:open="emit('close')">
    <template #body>
      <!-- Laid out like the settings: a framed list, the label on the left, the field on the right.
           Muted, not default: in dark the default border is the modal's own background (both neutral-800). -->
      <form id="saved-search" @submit.prevent="submit">
        <ul class="divide-y divide-muted rounded-lg border border-muted">
          <li class="flex flex-wrap items-center gap-x-4 gap-y-2 px-3 py-3 sm:px-4">
            <p class="w-28 shrink-0 text-sm">{{ $t('search.saved.name') }}</p>
            <UInput
              v-model="name"
              autofocus
              class="min-w-0 flex-1 max-sm:basis-full"
              :aria-label="$t('search.saved.name')"
            />
          </li>

          <li
            v-for="criterion in shown"
            :key="criterion"
            class="flex flex-wrap items-center gap-x-4 gap-y-2 px-3 py-3 sm:px-4"
          >
            <!-- The mode sits under the label, not beside the field: the chips of the field never push it out. -->
            <div class="flex w-28 shrink-0 flex-col items-start">
              <p class="text-sm">{{ labels[criterion] }}</p>
              <USelectMenu
                v-if="criterion === 'tags'"
                v-model="tagMode"
                :items="tagModeOptions"
                value-key="value"
                :search-input="false"
                size="xs"
                variant="ghost"
                class="-ms-2 text-muted"
                :ui="{ content: 'w-max' }"
                :aria-label="$t('search.saved.tagMode')"
              />
              <USelectMenu
                v-else-if="criterion === 'folders'"
                v-model="folderMode"
                :items="folderModeOptions"
                value-key="value"
                :search-input="false"
                size="xs"
                variant="ghost"
                class="-ms-2 text-muted"
                :aria-label="$t('search.saved.folderMode')"
              />
            </div>
            <!-- On a phone the field goes on its own line, under the label and the remove button. -->
            <div class="min-w-0 flex-1 max-sm:order-last max-sm:basis-full">
              <UInput
                v-if="criterion === 'text'"
                v-model="text"
                class="w-full"
                :placeholder="$t('search.saved.textPlaceholder')"
                :aria-label="labels.text"
              />
              <UInputMenu
                v-else-if="criterion === 'tags'"
                v-model="tagIds"
                multiple
                open-on-click
                :items="tagOptions"
                value-key="value"
                class="w-full"
                :placeholder="$t('search.saved.pickTags')"
                :aria-label="labels.tags"
                :ui="{ tagsItem: 'max-w-full' }"
              />
              <UInputMenu
                v-else-if="criterion === 'folders'"
                v-model="folderIds"
                multiple
                open-on-click
                :items="folderOptions"
                value-key="value"
                class="w-full"
                :placeholder="$t('search.saved.pickFolders')"
                :aria-label="labels.folders"
                :ui="{ tagsItem: 'max-w-full' }"
              />
              <DateRangeInput
                v-else-if="criterion === 'created'"
                v-model:from="createdFrom"
                v-model:to="createdTo"
                :label="labels.created"
              />
              <DateRangeInput v-else v-model:from="editedFrom" v-model:to="editedTo" :label="labels.edited" />
            </div>
            <UButton
              :icon="icons.close"
              color="neutral"
              variant="ghost"
              class="-mx-2 max-sm:ms-auto"
              :aria-label="$t('search.saved.removeCriterion', { name: labels[criterion] })"
              @click="remove(criterion)"
            />
          </li>

          <li v-if="addMenu.length" class="px-2 py-1.5">
            <UDropdownMenu :items="addMenu" :content="{ align: 'start' }">
              <UButton :icon="icons.add" :label="$t('search.saved.addCriterion')" color="neutral" variant="ghost" />
            </UDropdownMenu>
          </li>
        </ul>
      </form>
    </template>
    <template #footer>
      <div class="flex w-full items-center gap-2">
        <!-- With a text criterion the total also counts the notes after the page that may match. -->
        <span class="me-auto text-sm" :class="status === 'pending' ? 'text-dimmed' : 'text-muted'" aria-live="polite">
          {{ $t(criteria.text ? 'search.saved.matchesAtMost' : 'search.saved.matches', total) }}
        </span>
        <UButton :label="$t('common.cancel')" color="neutral" variant="ghost" @click="emit('close')" />
        <UButton type="submit" form="saved-search" :label="$t('settings.save')" :disabled="!name.trim()" />
      </div>
    </template>
  </UModal>
</template>
