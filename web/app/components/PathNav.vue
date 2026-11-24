<script setup lang="ts">
/** The folder path, '/A/B', as a breadcrumb, with the history's back and forward buttons. A page outside the folders passes its own crumbs. */
const props = defineProps<{ path?: string; crumbs?: { label: string; to?: string }[] }>();

const { icons } = useIcons();
const { t } = useI18n();

const destinations = useDestinations();

// TODO: matches folders by path, a folder name holding '/' gets no link; ask the API for the ancestors' ids if that matters.
const breadcrumb = computed(() => {
  if (props.crumbs) return props.crumbs;
  const byPath = new Map(destinations.value.map((folder) => [folder.label, folder.value]));
  const names = (props.path ?? '').split('/').filter(Boolean);
  return [
    { label: t('common.notes'), to: '/' },
    ...names.map((name, index) => {
      const folderId = byPath.get(`/${names.slice(0, index + 1).join('/')}`);
      return { label: name, to: folderId && `/folders/${folderId}` };
    }),
  ];
});

const router = useRouter();
const route = useRoute();
// vue-router keeps the neighbours of the current entry in history.state; route.fullPath makes it re-read on each navigation.
const canGoBack = computed(() => route.fullPath && !!window.history.state?.back);
const canGoForward = computed(() => route.fullPath && !!window.history.state?.forward);
</script>

<template>
  <!-- Plain muted text like the other pages' subtitles: no crumb is styled active, the path reads as before, its parts just link. -->
  <div class="flex min-w-0 items-center gap-1 text-xs text-muted">
    <UButton
      :icon="icons.back"
      color="neutral"
      variant="link"
      size="xs"
      class="shrink-0 p-0 text-muted"
      :aria-label="$t('path.back')"
      :disabled="!canGoBack"
      @click="router.back()"
    />
    <UButton
      :icon="icons.forward"
      color="neutral"
      variant="link"
      size="xs"
      class="mr-1 shrink-0 p-0 text-muted"
      :aria-label="$t('path.forward')"
      :disabled="!canGoForward"
      @click="router.forward()"
    />
    <UBreadcrumb
      :items="breadcrumb.map((crumb) => ({ ...crumb, active: false }))"
      :aria-label="$t('path.breadcrumb')"
      :ui="{
        list: 'gap-1',
        link: 'text-xs font-normal',
        separator: 'text-muted',
      }"
    >
      <template #separator><span>/</span></template>
    </UBreadcrumb>
  </div>
</template>
