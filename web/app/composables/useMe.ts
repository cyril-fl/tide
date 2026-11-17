import type { MeInput, User } from '~/types/items';

/** The account, loaded once for the whole app: every row of a folder asks for it, through usePro. */
export function useMe() {
  const api = useApi();
  return useLazyAsyncData('me', () => api<User>('/api/me'), {
    // Nuxt's default cancels the pending GET and sends another one for each new caller: they share it instead.
    dedupe: 'defer',
    // Lazy, each component fetches again once mounted: it takes the account already loaded. A refresh still fetches.
    // The shared data first, as useNuxtData reads it: me.value = … after a PATCH does not reach the payload.
    getCachedData: (key, nuxtApp, { cause }) =>
      cause === 'initial' ? (nuxtApp._asyncData[key]?.data.value ?? nuxtApp.payload.data[key]) : undefined,
  });
}

/**
 * The account before a page renders, kept as the layout's me; none without a session (the layout then sends to /login).
 * Not useApi: its useI18n needs a component, which a route middleware has not.
 */
export async function loadMe() {
  const cached = useNuxtData<User>('me').data;
  cached.value ??= await $fetch<User>('/api/me', {
    baseURL: useRuntimeConfig().public.apiBase,
    credentials: 'include',
  }).catch(() => undefined);
  return cached.value;
}

/**
 * Writes one change of the account through PATCH /api/me, guarded by its updated_at (utils/editMe): for the whole lists
 * (snippets, saved searches), which would otherwise drop what an agent or another tab saved meanwhile. edit makes the
 * body from the account, so on a 409 the change is redone on the fresh one; a second 409 keeps the fresh one and tells.
 */
export function useEditMe() {
  const api = useApi();
  const toast = useToast();
  const { t } = useI18n();
  const me = useNuxtData<User>('me').data;

  return async function (edit: (me: User) => MeInput) {
    const read = () => api<User>('/api/me');
    const result = await editMe(
      me.value ?? (await read()),
      edit,
      (body) => api<User>('/api/me', { method: 'PATCH', body, quiet: [409] }),
      read,
    );
    me.value = result.me;
    if (!result.saved) toast.add({ title: t('api.staleSettings'), color: 'error' });
  };
}
