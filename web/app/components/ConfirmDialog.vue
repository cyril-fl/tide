<script setup lang="ts">
// expected: a text to type before the action is enabled, for what cannot be undone. alternative: a second way out,
// answered 'alternative'.
const props = defineProps<{
  title: string;
  description?: string;
  action: string;
  expected?: string;
  alternative?: string;
}>();
const emit = defineEmits<{ close: [confirmed?: boolean | 'alternative'] }>();

const typed = ref('');
const disabled = computed(() => !!props.expected && typed.value !== props.expected);
</script>

<template>
  <UModal
    :title="title"
    :description="description"
    :ui="{ content: 'max-w-sm' }"
    :close="false"
    @update:open="emit('close')"
  >
    <template v-if="expected" #body>
      <UFormField :label="$t('common.typeToConfirm', { text: expected })">
        <UInput v-model="typed" autofocus class="w-full" autocomplete="off" />
      </UFormField>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton :label="$t('common.cancel')" color="neutral" variant="ghost" @click="emit('close')" />
        <UButton
          v-if="alternative"
          :label="alternative"
          color="neutral"
          variant="outline"
          @click="emit('close', 'alternative')"
        />
        <UButton :label="action" color="error" :disabled="disabled" @click="emit('close', true)" />
      </div>
    </template>
  </UModal>
</template>
