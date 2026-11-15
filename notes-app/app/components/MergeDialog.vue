<script setup lang="ts">
// Before a fork is merged into the note open, like a merge request: what the note's text becomes, and whether the
// fork goes to the trash after it. remember: the choice made the account's after_merge, the dialog no longer shown.
const props = defineProps<{ fork: string; original: string; before: string; after: string; trash: boolean }>();
const emit = defineEmits<{ close: [choice?: { trash: boolean; remember: boolean }] }>();

const trash = ref(props.trash);
const remember = ref(false);
const changes = computed(() => lineChanges(props.before, props.after));
const changed = computed(() => changes.value.some((change) => change.kind !== 'same'));
</script>

<template>
  <UModal
    :title="$t('fork.dialogTitle', { fork })"
    :description="$t('fork.dialogDescription', { original })"
    :ui="{ content: 'max-w-2xl' }"
    @update:open="emit('close')"
  >
    <template #body>
      <p class="mb-2 text-xs font-medium text-muted">{{ $t('fork.preview') }}</p>
      <div v-if="changed" class="max-h-[50vh] overflow-y-auto rounded-md border border-default p-2 text-sm">
        <component
          :is="{ same: 'span', removed: 'del', added: 'ins' }[change.kind]"
          v-for="(change, index) in changes"
          :key="index"
          class="block whitespace-pre-wrap"
          :class="
            { same: 'text-muted', removed: 'bg-error/10 text-error', added: 'bg-success/10 text-success' }[change.kind]
          "
          >{{ change.lines.join('\n') }}</component
        >
      </div>
      <p v-else class="text-sm text-muted">{{ $t('fork.noChange') }}</p>
      <div class="mt-4 space-y-2">
        <UCheckbox v-model="trash" :label="$t('fork.trash')" />
        <UCheckbox v-model="remember" :label="$t('fork.remember')" />
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton :label="$t('common.cancel')" color="neutral" variant="ghost" @click="emit('close')" />
        <UButton :label="$t('fork.confirm')" @click="emit('close', { trash, remember })" />
      </div>
    </template>
  </UModal>
</template>
