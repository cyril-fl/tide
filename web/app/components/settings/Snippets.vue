<script setup lang="ts">
import type { Snippet } from '~/types/items';

const { icons } = useIcons();
const { t } = useI18n();
const { data: me } = useMe();
const { confirm, snippet: editSnippet } = useDialogs();

// The snippet whose text shows under its row, one at a time.
const open = ref<string>();

// The API takes the whole list: an edit sends it back with the one changed. change makes it from the account's, and
// runs again on the fresh one after a 409; a snippet is found there by its name, as it has no id.
const editMe = useEditMe();
function saveSnippets(change: (snippets: Snippet[]) => Snippet[]) {
  return editMe((account) => ({ snippets: change([...(account.snippets ?? [])]) }));
}

async function addSnippet() {
  const added = await editSnippet({ title: t('settings.addSnippet') });
  if (added) await saveSnippets((snippets) => [...snippets, added]);
}

async function changeSnippet(old: Snippet) {
  const changed = await editSnippet({ title: t('settings.editSnippet'), value: old });
  if (!changed) return;
  await saveSnippets((snippets) => {
    const at = snippets.findIndex((s) => s.name === old.name);
    // Its created_at goes along, so a rename keeps the date the server gave it.
    snippets.splice(at < 0 ? snippets.length : at, 1, { ...old, ...changed });
    return snippets;
  });
}

async function removeSnippet(old: Snippet) {
  const confirmed = await confirm({
    title: t('settings.deleteSnippet', { name: old.name }),
    action: t('common.delete'),
  });
  if (!confirmed) return;
  await saveSnippets((snippets) => {
    const at = snippets.findIndex((s) => s.name === old.name);
    if (at >= 0) snippets.splice(at, 1);
    return snippets;
  });
}
</script>

<template>
  <ul class="divide-y divide-default rounded-lg border border-default">
    <li class="flex flex-wrap items-center gap-4 px-4 py-3">
      <div class="flex-1 basis-48">
        <p class="text-sm">{{ $t('settings.snippets') }}</p>
        <p class="text-xs text-muted">{{ $t('settings.snippetsDescription') }}</p>
      </div>
      <UButton
        :label="$t('settings.addSnippet')"
        :ui="{ label: 'max-sm:sr-only' }"
        :icon="icons.noteAdd"
        color="neutral"
        variant="outline"
        :disabled="!me"
        @click="addSnippet"
      />
    </li>
    <li v-for="snippet in me?.snippets" :key="snippet.name">
      <div class="flex items-center gap-3 px-4 py-2 transition-colors hover:bg-elevated/50">
        <button
          type="button"
          class="flex min-w-0 flex-1 items-center gap-3 self-stretch text-start"
          :aria-expanded="open === snippet.name"
          @click="open = open === snippet.name ? undefined : snippet.name"
        >
          <UIcon
            :name="icons.snippet"
            class="size-4 shrink-0 transition-colors"
            :class="open === snippet.name ? 'text-primary' : 'text-muted'"
          />
          <span class="min-w-0 flex-1 truncate text-sm">{{ snippet.name }}</span>
        </button>
        <UButton
          :icon="icons.rename"
          color="neutral"
          variant="ghost"
          :aria-label="$t('settings.editSnippet')"
          @click="changeSnippet(snippet)"
        />
        <UButton
          :icon="icons.delete"
          color="neutral"
          variant="ghost"
          :aria-label="$t('common.delete')"
          @click="removeSnippet(snippet)"
        />
      </div>
      <!-- The text as a note shows it, read only; a long one scrolls. -->
      <div v-if="open === snippet.name" class="flex max-h-80 flex-col border-t border-default bg-elevated/25">
        <NoteWriter :model-value="snippet.content" readonly compact :label="snippet.name" />
      </div>
    </li>
    <li v-if="me && !me.snippets.length" class="px-4 py-3 text-xs text-muted">
      {{ $t('settings.snippetsEmpty') }}
    </li>
  </ul>
</template>
