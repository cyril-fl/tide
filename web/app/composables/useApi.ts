import type { ToastProps } from '@nuxt/ui';
import type { FetchError, FetchOptions } from 'ofetch';

type ApiOptions = FetchOptions<'json'> & {
  /** What the toast says on a 409, the API's messages being in English: one text, or one per part of the API's message. */
  conflict?: string | Record<string, string>;
  /** The buttons of the toast on a 409. */
  conflictActions?: ToastProps['actions'];
  /** What the toast says on a 400, by part of the API's message. */
  invalid?: Record<string, string>;
  /** What the toast says on a 413, the limit of this call; a general text otherwise. */
  tooLarge?: string;
  /** The page shows the error itself: always, or for these statuses. */
  quiet?: boolean | number[];
};

function errorMessage(
  error: FetchError,
  t: (key: string) => string,
  { conflict, invalid, tooLarge }: ApiOptions,
): string {
  if (!error.status) return t('api.down');
  if (error.status === 413) return tooLarge ?? t('api.tooLarge');
  const message: string = error.data?.error ?? t('api.failed');
  const texts = { 400: invalid, 403: { 'plan of this account': t('api.notInPlan') }, 409: conflict }[error.status];
  if (!texts) return message;
  if (typeof texts === 'string') return texts;
  return Object.entries(texts).find(([part]) => message.includes(part))?.[1] ?? message;
}

/** Calls the Go API and tells the user when it fails; without a session, opens the login page. */
export function useApi() {
  const toast = useToast();
  const { t } = useI18n();
  const route = useRoute();
  const baseURL = useRuntimeConfig().public.apiBase;

  return async function api<T>(
    path: string,
    { conflict, conflictActions, invalid, tooLarge, quiet, ...options }: ApiOptions = {},
  ): Promise<T> {
    try {
      // include: in development the API is on another port, and its session cookie must go along.
      return await $fetch<T>(path, { baseURL, credentials: 'include', ...options } as object);
    } catch (error) {
      const status = (error as FetchError).status;
      if (status === 401 && route.path !== '/login') {
        await navigateTo({ path: '/login', query: { redirect: route.fullPath } });
      } else if (quiet !== true && !(Array.isArray(quiet) && quiet.includes(status ?? 0))) {
        toast.add({
          title: errorMessage(error as FetchError, t, { conflict, invalid, tooLarge }),
          color: 'error',
          ...(status === 409 && conflictActions && { actions: conflictActions, duration: 0 }),
        });
      }
      throw error;
    }
  };
}

type UploadOptions = Pick<ApiOptions, 'conflict' | 'invalid' | 'tooLarge'> & {
  body: XMLHttpRequestBodyInit;
  headers?: Record<string, string>;
  query?: Record<string, string>;
  /** The bytes sent so far, of total; loaded === total once the server has it all and works on it. */
  onProgress?: (loaded: number, total: number) => void;
  signal?: AbortSignal;
};

/** No byte sent for this long: the connection is taken for lost and the upload stopped. */
export const UPLOAD_STALL = 30_000;

/** The error of an upload stopped by its signal (the user) or by UPLOAD_STALL, never shown as a toast. */
export class UploadStopped extends Error {
  constructor(readonly stalled: boolean) {
    super(stalled ? 'stalled' : 'aborted');
  }
}

/**
 * Sends a big body to the API with its progress, which fetch cannot give, and the errors of useApi. It stops when no
 * byte goes out for UPLOAD_STALL (a lost connection), or on signal, throwing UploadStopped: the caller tells why.
 */
export function useUpload() {
  const toast = useToast();
  const { t } = useI18n();
  const route = useRoute();
  const baseURL = useRuntimeConfig().public.apiBase;

  return function upload<T>(path: string, { body, headers, query, onProgress, signal, ...texts }: UploadOptions) {
    return new Promise<T>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const url = new URL(path, baseURL || window.location.origin);
      for (const [key, value] of Object.entries(query ?? {})) url.searchParams.set(key, value);
      xhr.open('POST', url);
      xhr.withCredentials = true;
      for (const [key, value] of Object.entries(headers ?? {})) xhr.setRequestHeader(key, value);
      xhr.responseType = 'json';

      let stalled = false;
      let timer: ReturnType<typeof setTimeout> | undefined;
      // Reset at each progress; cleared once all is sent: the server's work has no byte to show.
      const watch = () => {
        clearTimeout(timer);
        timer = setTimeout(() => {
          stalled = true;
          xhr.abort();
        }, UPLOAD_STALL);
      };
      xhr.upload.onprogress = (event) => {
        watch();
        onProgress?.(event.loaded, event.total);
      };
      xhr.upload.onload = () => clearTimeout(timer);
      const stop = () => xhr.abort();
      signal?.addEventListener('abort', stop);
      const done = () => {
        clearTimeout(timer);
        signal?.removeEventListener('abort', stop);
      };

      xhr.onabort = () => {
        done();
        reject(new UploadStopped(stalled));
      };
      xhr.onerror = () => {
        done();
        toast.add({ title: t('api.down'), color: 'error' });
        reject(new Error('network'));
      };
      xhr.onload = async () => {
        done();
        if (xhr.status < 400) return resolve(xhr.response as T);
        if (xhr.status === 401 && route.path !== '/login') {
          await navigateTo({ path: '/login', query: { redirect: route.fullPath } });
        } else {
          const error = { status: xhr.status, data: xhr.response } as FetchError;
          toast.add({ title: errorMessage(error, t, texts), color: 'error' });
        }
        reject(new Error(String(xhr.status)));
      };
      watch();
      xhr.send(body);
    });
  };
}
