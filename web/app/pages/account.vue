<script setup lang="ts">
import type { FetchError } from 'ofetch';
import type { ApiToken, NewApiToken, NoteFile, User } from '~/types/items';

const { icons } = useIcons();
const { t, locale } = useI18n();

const api = useApi();
const toast = useToast();
const logout = useLogout();
const { confirm } = useDialogs();
const baseURL = useRuntimeConfig().public.apiBase;
const input = useTemplateRef('input');

const { data: me } = useMe();
// The advanced mode's settings nav leads back on its own: the plain settings page needs the footer's.
const { has } = usePro();

const avatarSrc = computed(() => me.value?.avatar && imageSrc(`/api/files/${me.value.avatar}`, baseURL));
const uploading = ref(false);

// square crops the image to its centered square and scales it to size: a phone photo of several MB becomes a few KB.
// An animated GIF keeps its first frame.
async function square(file: File, size: number): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const side = Math.min(bitmap.width, bitmap.height);
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = Math.min(side, size);
  canvas
    .getContext('2d')!
    .drawImage(
      bitmap,
      (bitmap.width - side) / 2,
      (bitmap.height - side) / 2,
      side,
      side,
      0,
      0,
      canvas.width,
      canvas.height,
    );
  bitmap.close();
  // A browser that cannot encode WebP gives a PNG.
  return new Promise((resolve, reject) =>
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('canvas.toBlob'))), 'image/webp', 0.9),
  );
}

async function changeAvatar(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  uploading.value = true;
  try {
    // A file the browser cannot decode fails here, silently: the API's own errors are already told by useApi().
    const image = await square(file, 256).catch(() => null);
    if (!image) {
      toast.add({ title: t('note.imageFailed', { name: file.name }), color: 'error' });
      return;
    }
    const name = `avatar.${image.type === 'image/webp' ? 'webp' : 'png'}`;
    const saved = await api<NoteFile>('/api/files', { method: 'POST', body: image, query: { name } });
    me.value = await api<User>('/api/me', { method: 'PATCH', body: { avatar: saved.id } });
  } finally {
    uploading.value = false;
    if (input.value) input.value.value = ''; // The same file can be picked again; the page may be gone.
  }
}

async function removeAvatar() {
  me.value = await api<User>('/api/me', { method: 'PATCH', body: { avatar: '' } });
}

const name = ref('');
watch(
  () => me.value?.name,
  (current) => (name.value = current ?? ''),
  { immediate: true },
);

async function rename() {
  me.value = await api<User>('/api/me', {
    method: 'PATCH',
    body: { name: name.value },
    conflict: t('account.nameTaken'),
  });
  toast.add({ title: t('account.renamed'), color: 'success' });
}

const current = ref('');
const next = ref('');

async function changePassword() {
  try {
    await api('/api/me/password', { method: 'PATCH', body: { current: current.value, new: next.value }, quiet: true });
  } catch (error) {
    if ((error as FetchError).status === 429) {
      toast.add({ title: t('api.tooManyTries', { wait: retryWait(error as FetchError) }), color: 'error' });
      return;
    }
    // The API's messages are in English: the two the user can fix are said here.
    const message: string = (error as FetchError).data?.error ?? '';
    const key = message.includes('current password')
      ? 'account.wrongPassword'
      : message.includes('characters')
        ? 'account.shortPassword'
        : 'api.failed';
    toast.add({ title: t(key), color: 'error' });
    return;
  }
  current.value = next.value = '';
  toast.add({ title: t('account.passwordChanged'), color: 'success' });
}
const { data: tokens, refresh: refreshTokens } = useLazyAsyncData('api-tokens', () =>
  api<ApiToken[]>('/api/me/tokens'),
);
const tokenName = ref('');
const tokenReadOnly = ref(false);
// As createapitoken.Lifetimes, 90d by default.
const lifetime = ref('90d');
const lifetimes = computed(() =>
  ['7d', '30d', '90d', '1y', 'never'].map((value) => ({ label: t(`account.tokenLifetimes.${value}`), value })),
);
// The new token, shown until the page is left: the API never gives it again.
const created = ref<NewApiToken | null>(null);

async function createToken() {
  created.value = await api<NewApiToken>('/api/me/tokens', {
    method: 'POST',
    body: { name: tokenName.value, expires_in: lifetime.value, read_only: tokenReadOnly.value },
  });
  tokenName.value = '';
  tokenReadOnly.value = false;
  await refreshTokens();
  await copyToken(created.value.token);
}

async function copyToken(token: string) {
  // Over plain HTTP, or without the permission, the clipboard refuses: the token stays readable in the page.
  const copied = await navigator.clipboard.writeText(token).then(
    () => true,
    () => false,
  );
  if (copied) toast.add({ title: t('account.tokenCopied'), color: 'success' });
}

async function revokeToken(token: ApiToken) {
  const confirmed = await confirm({
    title: t('account.revokeToken', { name: token.name }),
    action: t('account.revoke'),
  });
  if (!confirmed) return;
  await api(`/api/me/tokens/${token.id}`, { method: 'DELETE' });
  if (created.value?.api_token.id === token.id) created.value = null;
  await refreshTokens();
}

function day(at: string) {
  return new Date(at).toLocaleDateString(locale.value);
}
</script>

<template>
  <div class="flex min-h-full flex-col p-8">
    <PageHeader :title="$t('account.title')" />

    <section class="mb-8">
      <h2 class="mb-1 text-sm font-semibold">{{ $t('account.profile') }}</h2>
      <ul>
        <li class="flex flex-wrap items-center gap-4 border-b border-muted py-3">
          <UAvatar :src="avatarSrc || undefined" :alt="me?.name" size="3xl" />
          <div class="flex-1 basis-48">
            <p class="text-sm">{{ $t('account.avatar') }}</p>
            <p class="text-xs text-muted">{{ $t('account.avatarDescription') }}</p>
          </div>
          <input
            ref="input"
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            class="hidden"
            @change="changeAvatar"
          />
          <UButton
            v-if="me?.avatar"
            :label="$t('account.removeAvatar')"
            :icon="icons.delete"
            :ui="{ label: 'max-sm:sr-only' }"
            color="neutral"
            variant="ghost"
            @click="removeAvatar"
          />
          <UButton
            :label="$t('account.chooseAvatar')"
            :ui="{ label: 'max-sm:sr-only' }"
            :icon="icons.avatar"
            color="neutral"
            variant="outline"
            :loading="uploading"
            @click="input?.click()"
          />
        </li>
        <li class="py-3">
          <form class="flex flex-wrap items-center gap-4" @submit.prevent="rename">
            <div class="flex-1 basis-48">
              <p class="text-sm">{{ $t('account.name') }}</p>
              <p class="text-xs text-muted">{{ $t('account.nameDescription') }}</p>
            </div>
            <UInput v-model="name" autocomplete="username" :aria-label="$t('account.name')" class="w-56" />
            <UButton
              type="submit"
              :label="$t('account.save')"
              :icon="icons.save"
              :ui="{ label: 'max-sm:sr-only' }"
              color="neutral"
              variant="outline"
              :disabled="!name.trim() || name.trim() === me?.name"
            />
          </form>
        </li>
      </ul>
    </section>

    <section class="mb-8">
      <h2 class="mb-1 text-sm font-semibold">{{ $t('account.password') }}</h2>
      <p class="mb-3 text-xs text-muted">{{ $t('account.passwordDescription') }}</p>
      <form class="flex flex-wrap items-end gap-4" @submit.prevent="changePassword">
        <UFormField :label="$t('account.currentPassword')">
          <UInput v-model="current" type="password" autocomplete="current-password" class="w-56" />
        </UFormField>
        <UFormField :label="$t('account.newPassword')">
          <UInput v-model="next" type="password" autocomplete="new-password" class="w-56" />
        </UFormField>
        <UButton
          type="submit"
          :label="$t('account.changePassword')"
          color="neutral"
          variant="outline"
          :disabled="!current || !next"
        />
      </form>
    </section>

    <section class="mb-8">
      <h2 class="mb-1 text-sm font-semibold">{{ $t('account.tokens') }}</h2>
      <p class="mb-3 text-xs text-muted">{{ $t('account.tokensDescription') }} {{ $t('account.tokensOAuth') }}</p>
      <form class="flex flex-wrap items-end gap-4" @submit.prevent="createToken">
        <UFormField :label="$t('account.tokenName')">
          <UInput v-model="tokenName" :placeholder="$t('account.tokenNamePlaceholder')" class="w-56" />
        </UFormField>
        <UFormField :label="$t('account.tokenExpiresIn')">
          <USelectMenu v-model="lifetime" :items="lifetimes" value-key="value" :search-input="false" class="w-40" />
        </UFormField>
        <UCheckbox v-model="tokenReadOnly" :label="$t('account.tokenReadOnly')" class="mb-2" />
        <UButton
          type="submit"
          :label="$t('account.createToken')"
          :icon="icons.apiToken"
          color="neutral"
          variant="outline"
          :disabled="!tokenName.trim()"
        />
      </form>

      <div v-if="created" class="mt-3 flex flex-wrap items-center gap-3 rounded-lg border border-default px-4 py-2">
        <span class="text-xs text-muted">{{ $t('account.newToken', { name: created.api_token.name }) }}</span>
        <UInput
          :model-value="created.token"
          readonly
          size="sm"
          class="min-w-0 flex-1 basis-64 font-mono"
          :aria-label="$t('account.newToken', { name: created.api_token.name })"
        >
          <template #trailing>
            <UButton
              :icon="icons.copy"
              :aria-label="$t('account.copyToken')"
              color="neutral"
              variant="link"
              size="sm"
              @click="copyToken(created.token)"
            />
          </template>
        </UInput>
      </div>

      <ul v-if="tokens?.length" class="mt-3 divide-y divide-default rounded-lg border border-default">
        <li v-for="token in tokens" :key="token.id" class="flex flex-wrap items-center gap-3 px-4 py-2">
          <UIcon :name="icons.apiToken" class="size-4 shrink-0 text-muted" />
          <div class="min-w-0 flex-1 basis-48">
            <p class="flex min-w-0 items-center gap-2 text-sm">
              <span class="truncate">{{ token.name }}</span>
              <UBadge v-if="token.oauth" label="OAuth" size="sm" color="neutral" variant="subtle" />
              <UBadge
                v-if="token.read_only"
                :label="$t('account.tokenReadOnly')"
                size="sm"
                color="neutral"
                variant="subtle"
              />
            </p>
            <p class="text-xs text-muted">
              {{
                $t('account.tokenDates', {
                  created: day(token.created_at),
                  used: token.last_used_at
                    ? $t('account.tokenUsed', { date: day(token.last_used_at) })
                    : $t('account.tokenNeverUsed'),
                  expires: token.expires_at
                    ? $t('account.tokenExpires', { date: day(token.expires_at) })
                    : $t('account.tokenNoExpiry'),
                })
              }}
            </p>
          </div>
          <UButton
            :label="$t('account.revoke')"
            :ui="{ label: 'max-sm:sr-only' }"
            :icon="icons.delete"
            color="neutral"
            variant="ghost"
            @click="revokeToken(token)"
          />
        </li>
      </ul>
      <p v-else-if="tokens" class="mt-3 text-xs text-muted">{{ $t('account.tokensEmpty') }}</p>
    </section>

    <section>
      <h2 class="mb-1 text-sm font-semibold">{{ $t('account.session') }}</h2>
      <div class="flex flex-wrap items-center gap-4 py-3">
        <p class="min-w-0 flex-1 truncate text-sm">
          {{ $t('settings.loggedInAs') }} <span class="font-medium">{{ me?.name }}</span>
        </p>
        <UButton
          :label="$t('settings.logout')"
          :ui="{ label: 'max-sm:sr-only' }"
          :icon="icons.logout"
          color="neutral"
          variant="outline"
          @click="logout"
        />
      </div>
    </section>

    <!-- The footer of the notes and folders: the advanced mode's settings nav leads back on its own. -->
    <footer
      v-if="!has('settingsNav')"
      class="sticky bottom-0 z-10 -mx-8 -mb-8 mt-auto border-t border-default bg-default px-8"
    >
      <div class="flex h-9 items-center text-xs text-muted">
        <PathNav :crumbs="[{ label: $t('common.settings'), to: '/settings' }, { label: $t('account.title') }]" />
      </div>
    </footer>
  </div>
</template>
