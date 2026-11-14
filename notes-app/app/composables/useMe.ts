import type { User } from '~/types/items';

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
